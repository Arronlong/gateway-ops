import json
from pathlib import Path
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[1]

class WindowsLauncherTests(unittest.TestCase):
    def test_bat_delegates_to_relative_script_without_global_kills(self):
        text = (ROOT / "start-test.bat").read_text(encoding="utf-8")
        self.assertIn('%~dp0tools\\restart-test.ps1', text)
        self.assertNotIn("taskkill", text.lower())
        self.assertNotIn("WindowsTerminal.exe", text)

    def test_dry_run_is_isolated_and_does_not_launch(self):
        completed = subprocess.run(
            ["powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File",
             str(ROOT / "tools/restart-test.ps1"), "-DryRun"],
            cwd=ROOT, capture_output=True, encoding="utf-8", errors="strict", check=True,
        )
        plan = json.loads(completed.stdout)
        self.assertEqual(plan["FrontendPort"], 3014)
        self.assertEqual(plan["BackendPort"], 8422)
        self.assertEqual(plan["ProjectRoot"].lower(), str(ROOT).lower())
        self.assertEqual(plan["Branch"], "main")
        self.assertRegex(plan["TestSchema"], r"^gatewayops_pelican_test_[a-f0-9]{12}$")
        self.assertIn("public schema is untouched", plan["DatabaseNote"])
        self.assertIn("/relay-stations", plan["URL"])

    def test_stop_requires_saved_process_identity(self):
        text = (ROOT / "tools/restart-test.ps1").read_text(encoding="utf-8-sig")
        self.assertIn("Get-ProcessRecord", text)
        self.assertIn("StartTime", text)
        self.assertIn("Stop-Process -Id", text)
        self.assertNotIn("taskkill", text.lower())
        self.assertNotIn("Stop-Process -Name", text)

if __name__ == "__main__":
    unittest.main()
