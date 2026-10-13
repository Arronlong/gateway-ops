"""统一镜像发布、版本校验和 GitHub Release 管理，仅依赖 Python 标准库。"""

import functools
import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone


VERSION_PATTERN = re.compile(r"[vV]?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?")


def normalize_version(value):
    match = VERSION_PATTERN.fullmatch(value)
    if not match:
        raise ValueError(f"Invalid semantic version: {value!r}")
    prerelease = match.group(4)
    if prerelease and any(part.isdigit() and len(part) > 1 and part.startswith("0") for part in prerelease.split(".")):
        raise ValueError(f"Invalid numeric prerelease identifier: {value!r}")
    return value[1:] if value[:1] in ("v", "V") else value


def compare_versions(left, right):
    def parts(value):
        match = VERSION_PATTERN.fullmatch(normalize_version(value))
        return tuple(int(match.group(index)) for index in (1, 2, 3)), match.group(4)
    a, pre_a = parts(left)
    b, pre_b = parts(right)
    if a != b:
        return (a > b) - (a < b)
    if pre_a == pre_b:
        return 0
    if pre_a is None or pre_b is None:
        return 1 if pre_a is None else -1
    for x, y in zip(pre_a.split("."), pre_b.split(".")):
        if x == y:
            continue
        if x.isdigit() and y.isdigit():
            return (int(x) > int(y)) - (int(x) < int(y))
        if x.isdigit() != y.isdigit():
            return -1 if x.isdigit() else 1
        return (x > y) - (x < y)
    return (len(pre_a.split(".")) > len(pre_b.split("."))) - (len(pre_a.split(".")) < len(pre_b.split(".")))


def prepare_build(ref, sha, run_id, attempt, created, git_tags=()):
    if not re.fullmatch(r"[0-9a-f]{40}", sha) or not run_id.isdigit() or not attempt.isdigit():
        raise ValueError("Invalid build revision or run identifier")
    result = {"revision": sha, "build_time": created, "build_tag": f"build-{sha[:12]}-{run_id}-{attempt}"}
    if ref.startswith("refs/tags/"):
        tag = ref.removeprefix("refs/tags/")
        if not tag.startswith(("v", "V")):
            raise ValueError("Release tags must start with v")
        version = normalize_version(tag)
        if "+" in version or compare_versions(version.split("-")[0], "1.0.0") < 0 or len(version) > 120:
            raise ValueError("Release versions must start at 1.0.0 and fit Docker tag syntax")
        result.update(version=version, mode="release", release_tag=tag, prerelease=str("-" in version).lower())
    elif ref == "refs/heads/main":
        stable = ["1.0.0"]
        for tag in git_tags:
            try:
                version = normalize_version(tag)
                if "-" not in version and "+" not in version:
                    stable.append(version)
            except ValueError:
                continue
        base = max(stable, key=functools.cmp_to_key(compare_versions))
        result.update(version=f"{base}-dev.sha.{sha[:12]}", mode="development", release_tag="", prerelease="false")
    else:
        raise ValueError("Only main or a version tag can publish images")
    return result


def publication_targets(repository, ghcr_image="", dockerhub_image="", dockerhub_username="", dockerhub_token=""):
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository):
        raise ValueError("Invalid GITHUB_REPOSITORY")
    ghcr_image = (ghcr_image or f"ghcr.io/{repository}").lower()
    if not re.fullmatch(r"ghcr\.io/[a-z0-9][a-z0-9._-]*(?:/[a-z0-9][a-z0-9._-]*)+", ghcr_image):
        raise ValueError("GHCR_IMAGE must be an untagged GHCR image name")
    images = [ghcr_image]
    if dockerhub_username and dockerhub_token:
        dockerhub_image = (dockerhub_image or f"{dockerhub_username}/{repository.split('/')[1]}").lower()
        if not re.fullmatch(r"(?:docker\.io/)?[a-z0-9][a-z0-9._-]*/[a-z0-9][a-z0-9._-]*", dockerhub_image):
            raise ValueError("DOCKERHUB_IMAGE must be an untagged Docker Hub image name")
        images.append(dockerhub_image)
    return images


def publication_plan(version, releases):
    version = normalize_version(version)
    if "-" in version:
        return {"latest": False}
    stable = [normalize_version(item["tag_name"]) for item in releases if not item.get("draft") and not item.get("prerelease")]
    stable = [item for item in stable if "-" not in item]
    return {"latest": all(compare_versions(version, existing) >= 0 for existing in stable)}


class GitHub:
    def __init__(self, repository, token):
        if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository) or not token:
            raise ValueError("GITHUB_REPOSITORY and GITHUB_TOKEN are required")
        self.base = f"https://api.github.com/repos/{repository}"
        self.token = token

    def request(self, path, method="GET", payload=None, missing_ok=False):
        request = urllib.request.Request(self.base + path, method=method,
            data=None if payload is None else json.dumps(payload).encode("utf-8"),
            headers={"Accept":"application/vnd.github+json", "Authorization":f"Bearer {self.token}",
                     "X-GitHub-Api-Version":"2022-11-28", "Content-Type":"application/json", "User-Agent":"GatewayOps-release"})
        try:
            with urllib.request.urlopen(request, timeout=20) as response:
                return json.load(response)
        except urllib.error.HTTPError as error:
            if missing_ok and error.code == 404:
                return None
            raise RuntimeError(f"GitHub API {method} {path} returned HTTP {error.code}") from None

    def releases(self):
        result = []
        for page in range(1, 101):
            batch = self.request(f"/releases?per_page=100&page={page}")
            if not isinstance(batch, list):
                raise RuntimeError("Invalid GitHub releases response")
            result.extend(batch)
            if len(batch) < 100:
                return result
        raise RuntimeError("Release list exceeds pagination limit; refusing to move latest")


class Registry:
    def inspect(self, reference):
        result = subprocess.run(["docker", "buildx", "imagetools", "inspect", reference], capture_output=True, text=True)
        if result.returncode:
            if re.search(r"manifest unknown|not found|does not exist|name unknown|\b404\b", result.stderr, re.I):
                return None
            raise RuntimeError(f"Cannot inspect {reference}: {result.stderr.strip()}")
        match = re.search(r"^Digest:\s+(sha256:[0-9a-f]{64})\s*$", result.stdout, re.M)
        if not match:
            raise RuntimeError(f"No manifest digest in inspect response for {reference}")
        return match.group(1)

    def tag(self, image, tag, digest):
        subprocess.run(["docker", "buildx", "imagetools", "create", "--tag", f"{image}:{tag}", f"{image}@{digest}"], check=True)
        if self.inspect(f"{image}:{tag}") != digest:
            raise RuntimeError(f"Digest verification failed for {image}:{tag}")


def pin_images(images, tags, built_digest, registry):
    # 已存在的固定标签优先，重跑工作流不能用重新构建的镜像覆盖发布版本。
    existing = {registry.inspect(f"{image}:{tag}") for image in images for tag in tags}
    existing.discard(None)
    if len(existing) > 1:
        raise RuntimeError("Immutable image tags already point to different digests")
    digest = next(iter(existing), built_digest)
    for image in images:
        if registry.inspect(f"{image}@{digest}") != digest:
            raise RuntimeError(f"Immutable release digest is missing from {image}; refusing to overwrite version tags")
        for tag in tags:
            if registry.inspect(f"{image}:{tag}") is None:
                registry.tag(image, tag, digest)
    return digest


def publish(build, built_digest, images, github, registry):
    if not re.fullmatch(r"sha256:[0-9a-f]{64}", built_digest):
        raise ValueError("Invalid build digest")
    if build["mode"] == "development":
        digest = pin_images(images, [f"sha-{build["revision"]}"], built_digest, registry)
        # 并发队列不保证顺序；只有 main 当前提交能推进 edge。
        if github.request("/commits/main")["sha"] == build["revision"]:
            for image in images:
                registry.tag(image, "edge", digest)
        return {"version":build["version"], "revision":build["revision"], "digest":digest, "latest":False}

    version = build["version"]
    digest = pin_images(images, [version, "v" + version], built_digest, registry)
    tag = build["release_tag"]
    release = github.request("/releases/tags/" + urllib.parse.quote(tag, safe=""), missing_ok=True)
    if release is None:
        release = github.request("/releases", "POST", {"tag_name":tag, "target_commitish":build["revision"], "name":"v"+version,
            "draft":True, "prerelease":build["prerelease"] == "true", "generate_release_notes":True, "make_latest":"false"})
    plan = publication_plan(version, github.releases())
    if plan["latest"]:
        for image in images:
            registry.tag(image, "latest", digest)
    # 镜像推送及标签核验成功后才公开 Release，使更新提示对应可拉取的镜像。
    github.request(f"/releases/{release["id"]}", "PATCH", {"draft":False, "prerelease":build["prerelease"] == "true",
        "make_latest":"true" if plan["latest"] else "false"})
    return {"version":version, "revision":build["revision"], "digest":digest, "latest":plan["latest"], "images":images}


def main():
    tags = subprocess.check_output(["git", "tag", "--merged", "HEAD"], text=True).splitlines()
    sha = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
    build = prepare_build(os.environ["GITHUB_REF"], sha, os.environ["GITHUB_RUN_ID"], os.environ["GITHUB_RUN_ATTEMPT"],
        datetime.now(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z"), tags)
    if sys.argv[1:] == ["prepare"]:
        images = publication_targets(os.environ["GITHUB_REPOSITORY"],
            os.environ.get("GHCR_IMAGE", ""), os.environ.get("DOCKERHUB_IMAGE", ""),
            os.environ.get("DOCKERHUB_USERNAME", ""), os.environ.get("DOCKERHUB_TOKEN", ""))
        # 构建、固定标签核验和 Release 发布使用同一组已启用的仓库。
        with open(os.environ["GITHUB_ENV"], "a", encoding="utf-8") as environment:
            environment.write(f"GHCR_IMAGE={images[0]}\nDOCKERHUB_ENABLED={str(len(images) > 1).lower()}\nIMAGES={' '.join(images)}\n")
            environment.write("IMAGE_TAGS<<GATEWAYOPS_IMAGE_TAGS\n")
            for image in images:
                environment.write(f"{image}:{build['build_tag']}\n")
            environment.write("GATEWAYOPS_IMAGE_TAGS\n")
        if len(images) == 1:
            print("::notice::未配齐 DOCKERHUB_USERNAME 和 DOCKERHUB_TOKEN，跳过 Docker Hub，仅发布 GHCR。")
        with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
            for key, value in build.items():
                if "\n" in value or "\r" in value:
                    raise ValueError("Multiline build output is not allowed")
                output.write(f"{key}={value}\n")
        print(json.dumps(build, ensure_ascii=False))
    elif sys.argv[1:] == ["publish"]:
        result = publish(build, os.environ["BUILD_DIGEST"], os.environ["IMAGES"].split(),
            GitHub(os.environ["GITHUB_REPOSITORY"], os.environ["GITHUB_TOKEN"]), Registry())
        print(json.dumps(result, ensure_ascii=False))
        if path := os.environ.get("GITHUB_STEP_SUMMARY"):
            with open(path, "a", encoding="utf-8") as output:
                output.write("```json\n"+json.dumps(result, indent=2, ensure_ascii=False)+"\n```\n")
    else:
        raise ValueError("Usage: release.py prepare|publish")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, RuntimeError, subprocess.CalledProcessError, urllib.error.URLError) as error:
        print(f"Release failed: {error}", file=sys.stderr)
        sys.exit(1)
