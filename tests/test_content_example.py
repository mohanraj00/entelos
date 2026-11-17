"""Run the content example tests and the commands shown to a new reader."""
import os
from pathlib import Path
import re
import shlex
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[1]


class ContentExample(unittest.TestCase):
    def run_command(self, command):
        result = subprocess.run(shlex.split(command), cwd=ROOT, capture_output=True,
                                text=True, timeout=120,
                                env=os.environ | {'GOPROXY': 'off', 'GOTOOLCHAIN': 'local'})
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return result.stdout

    def test_example(self):
        self.run_command('go -C examples/content-update test ./...')

    def test_readme_commands(self):
        command = 'go -C examples/content-update run .'
        for path in ['README.md', 'examples/content-update/README.md']:
            with self.subTest(path=path):
                blocks = re.findall(r'```sh\n(.*?)```', (ROOT / path).read_text(), re.S)
                self.assertIn(command, [block.strip() for block in blocks])
        output = self.run_command(command)
        self.assertIn('Approval refused: qualification_failed', output)
        self.assertRegex(output, r'Retained report: sha256:[0-9a-f]{64}, status failed\.')
        self.assertIn('6. Read at 2026-11-15:', output)
        self.assertIn('Read checks passed: item digests and sizes agree with the active manifest.', output)
        self.assertLess(output.index('3. Qualify: passed'), output.index('Retained report:'))
        self.assertLess(output.index('6. Read at 2026-11-15:'), output.index('Read checks passed:'))
        self.assertIn('returns: Return the sample device within 30 days.', output)


if __name__ == '__main__':
    unittest.main()
