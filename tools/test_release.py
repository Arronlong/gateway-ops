import io
import os
import tempfile
import unittest
from contextlib import redirect_stdout
from pathlib import Path
from unittest.mock import patch
from types import SimpleNamespace

from release import Registry, compare_versions, main, normalize_version, pin_images, prepare_build, publication_plan, publication_targets, publish


class FakeRegistry:
    def __init__(self, entries):
        self.entries = dict(entries)
        self.writes = []

    def inspect(self, reference):
        return self.entries.get(reference)

    def tag(self, image, tag, digest):
        self.entries[f"{image}:{tag}"] = digest
        self.writes.append((image, tag, digest))


class FakeGitHub:
    def __init__(self, releases=(), sha="a" * 40):
        self.items = list(releases)
        self.sha = sha
        self.writes = []

    def releases(self):
        return self.items

    def request(self, path, method="GET", payload=None, missing_ok=False):
        if path == "/commits/main":
            return {"sha":self.sha}
        if method == "GET":
            return None
        self.writes.append((path, method, payload))
        return {"id":123}


class ReleaseTests(unittest.TestCase):
    def test_missing_or_partial_hub_credentials_only_select_ghcr(self):
        for username, token in (("", ""), ("publisher", ""), ("", "token")):
            with self.subTest(username=username, token=bool(token)):
                self.assertEqual(publication_targets("Arronlong/gateway-ops",
                    dockerhub_username=username, dockerhub_token=token),
                    ["ghcr.io/arronlong/gateway-ops"])

    def test_complete_hub_credentials_enable_users_own_repository(self):
        self.assertEqual(publication_targets("Contributor/gateway-ops",
            dockerhub_username="publisher", dockerhub_token="token"),
            ["ghcr.io/contributor/gateway-ops", "publisher/gateway-ops"])

    def test_registry_variables_override_image_names(self):
        self.assertEqual(publication_targets("Contributor/gateway-ops",
            ghcr_image="ghcr.io/team/custom-app", dockerhub_image="team/custom-app",
            dockerhub_username="publisher", dockerhub_token="token"),
            ["ghcr.io/team/custom-app", "team/custom-app"])
        self.assertEqual(publication_targets("Contributor/gateway-ops",
            dockerhub_image="team/custom-app"), ["ghcr.io/contributor/gateway-ops"])

    def test_image_names_cannot_inject_environment_values_or_tags(self):
        for image in ("ghcr.io/team/app\nINJECTED=true", "ghcr.io/team/app:latest", "ghcr.io/team/app@sha256:abc", "team/app"):
            with self.subTest(image=image), self.assertRaises(ValueError):
                publication_targets("Contributor/gateway-ops", ghcr_image=image)
        for image in ("team/app\nINJECTED=true", "team/app:latest", "ghcr.io/team/app"):
            with self.subTest(image=image), self.assertRaises(ValueError):
                publication_targets("Contributor/gateway-ops", dockerhub_image=image,
                    dockerhub_username="publisher", dockerhub_token="token")

    def test_prepare_shares_selected_targets_with_build_and_publish(self):
        for enabled in (False, True):
            with self.subTest(enabled=enabled), tempfile.TemporaryDirectory() as directory:
                output = Path(directory) / "output"
                environment = Path(directory) / "environment"
                values = {"GITHUB_REF":"refs/tags/v1.0.0", "GITHUB_REPOSITORY":"Arronlong/gateway-ops",
                    "GITHUB_RUN_ID":"12", "GITHUB_RUN_ATTEMPT":"1", "GITHUB_OUTPUT":str(output),
                    "GITHUB_ENV":str(environment), "DOCKERHUB_USERNAME":"publisher" if enabled else "",
                    "DOCKERHUB_TOKEN":"sensitive-token" if enabled else ""}
                with patch.dict(os.environ, values, clear=True), patch("release.sys.argv", ["release.py", "prepare"]), \
                    patch("release.subprocess.check_output", side_effect=["v1.0.0\n", "a" * 40 + "\n"]), redirect_stdout(io.StringIO()) as log:
                    main()
                content = environment.read_text(encoding="utf-8")
                targets = ["ghcr.io/arronlong/gateway-ops"] + (["publisher/gateway-ops"] if enabled else [])
                self.assertIn("IMAGES=" + " ".join(targets) + "\n", content)
                self.assertIn("DOCKERHUB_ENABLED=" + str(enabled).lower() + "\n", content)
                for image in targets:
                    self.assertIn(image + ":build-aaaaaaaaaaaa-12-1\n", content)
                if not enabled:
                    self.assertNotIn("publisher/", content)
                self.assertIn("version=1.0.0\n", output.read_text(encoding="utf-8"))
                self.assertNotIn("sensitive-token", content + output.read_text(encoding="utf-8") + log.getvalue())

    def test_ghcr_only_release_publishes_fixed_tags_latest_and_release(self):
        digest = "sha256:" + "a" * 64
        image = "ghcr.io/arronlong/gateway-ops"
        build = prepare_build("refs/tags/v1.0.0", "a" * 40, "1", "1", "now")
        registry = FakeRegistry({image + "@" + digest:digest})
        github = FakeGitHub()
        result = publish(build, digest, [image], github, registry)
        self.assertEqual(result["images"], [image])
        self.assertTrue(result["latest"])
        self.assertEqual(registry.writes, [(image, tag, digest) for tag in ("1.0.0", "v1.0.0", "latest")])
        self.assertFalse(github.writes[-1][2]["draft"])
        self.assertEqual(github.writes[-1][2]["make_latest"], "true")

    def test_version_order_and_validation(self):
        self.assertEqual(normalize_version("V1.0.0"), "1.0.0")
        self.assertLess(compare_versions("1.2.0", "1.10.0"), 0)
        self.assertLess(compare_versions("1.0.0-rc.2", "1.0.0-rc.10"), 0)
        self.assertLess(compare_versions("1.0.0-rc.1", "1.0.0"), 0)
        self.assertEqual(compare_versions("1.0.0+a", "1.0.0+b"), 0)
        for bad in ("latest", "v1.0", "1.02.0", "1.0.0-01", "1.0.0\ninjected=true"):
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                normalize_version(bad)

    def test_builds_start_at_one_and_keep_development_separate(self):
        stable = prepare_build("refs/tags/v1.0.0", "a" * 40, "12", "1", "2026-10-13T00:00:00Z")
        self.assertEqual(stable["version"], "1.0.0")
        self.assertEqual(stable["mode"], "release")
        dev = prepare_build("refs/heads/main", "a" * 40, "12", "1", "now")
        self.assertEqual(dev["version"], "1.0.0-dev.sha.aaaaaaaaaaaa")
        self.assertEqual(dev["mode"], "development")
        self.assertEqual(prepare_build("refs/tags/V1.1.0-rc.1", "a" * 40, "12", "1", "now")["prerelease"], "true")
        for ref in ("refs/tags/v0.9.0", "refs/tags/v1.0.0+build.1", "refs/heads/feature"):
            with self.assertRaises(ValueError):
                prepare_build(ref, "a" * 40, "12", "1", "now")

    def test_latest_never_moves_backwards_or_to_prerelease(self):
        releases = [{"tag_name":"v1.10.0"}, {"tag_name":"v2.0.0", "draft":True}, {"tag_name":"v3.0.0-rc.1", "prerelease":True}]
        self.assertFalse(publication_plan("1.2.1", releases)["latest"])
        self.assertTrue(publication_plan("1.10.0", releases)["latest"])
        self.assertTrue(publication_plan("2.0.0", releases)["latest"])
        self.assertFalse(publication_plan("3.0.0-rc.1", releases)["latest"])

    def test_first_release_and_invalid_existing_versions(self):
        self.assertTrue(publication_plan("1.0.0", [])["latest"])
        with self.assertRaises(ValueError):
            publication_plan("1.0.0", [{"tag_name":"broken"}])

    def test_rerun_reuses_published_digest_in_both_registries(self):
        old, new = "sha256:" + "a" * 64, "sha256:" + "b" * 64
        registry = FakeRegistry({f"{image}:{tag}":old for image in ("hub/app", "ghcr/app") for tag in ("1.0.0", "v1.0.0")})
        registry.entries.update({f"{image}@{old}":old for image in ("hub/app", "ghcr/app")})
        self.assertEqual(pin_images(["hub/app", "ghcr/app"], ["1.0.0", "v1.0.0"], new, registry), old)
        self.assertEqual(registry.writes, [])

    def test_conflicting_fixed_tags_fail_without_writes(self):
        registry = FakeRegistry({"hub/app:1.0.0":"sha256:a", "ghcr/app:1.0.0":"sha256:b"})
        with self.assertRaises(RuntimeError):
            pin_images(["hub/app", "ghcr/app"], ["1.0.0"], "sha256:c", registry)
        self.assertEqual(registry.writes, [])

    def test_no_latest_for_old_or_prerelease_and_release_after_tags(self):
        digest = "sha256:" + "a" * 64
        for version, expected in (("1.0.1", False), ("1.3.0", True), ("2.0.0-rc.1", False)):
            with self.subTest(version=version):
                build = prepare_build("refs/tags/v"+version, "a" * 40, "1", "1", "now")
                registry = FakeRegistry({"hub/app@"+digest:digest, "ghcr/app@"+digest:digest})
                github = FakeGitHub([{"tag_name":"v1.2.0"}])
                result = publish(build, digest, ["hub/app", "ghcr/app"], github, registry)
                self.assertEqual(result["latest"], expected)
                self.assertEqual(sum(tag == "latest" for _, tag, _ in registry.writes), 2 if expected else 0)
                final = github.writes[-1][2]
                self.assertFalse(final["draft"])
                self.assertEqual(final["make_latest"], "true" if expected else "false")

    def test_old_main_build_does_not_move_edge(self):
        digest = "sha256:" + "a" * 64
        build = prepare_build("refs/heads/main", "a" * 40, "1", "1", "now")
        registry = FakeRegistry({"hub/app@"+digest:digest})
        github = FakeGitHub(sha="b" * 40)
        publish(build, digest, ["hub/app"], github, registry)
        self.assertEqual([tag for _, tag, _ in registry.writes], ["sha-"+"a"*40])
        self.assertEqual(github.writes, [])

    def test_registry_errors_are_not_treated_as_missing_tags(self):
        with patch("release.subprocess.run", return_value=SimpleNamespace(returncode=1, stdout="", stderr="authentication required")):
            with self.assertRaises(RuntimeError):
                Registry().inspect("hub/app:1.0.0")
        with patch("release.subprocess.run", return_value=SimpleNamespace(returncode=1, stdout="", stderr="manifest unknown")):
            self.assertIsNone(Registry().inspect("hub/app:1.0.0"))


if __name__ == "__main__":
    unittest.main()
