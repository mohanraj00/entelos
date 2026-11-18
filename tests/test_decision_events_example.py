"""Run the decision-event README commands, including a process restart."""
import os
from pathlib import Path
import re
import shlex
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class DecisionEventsExample(unittest.TestCase):
    def test_readme_commands(self):
        readme = (ROOT / 'examples/decision-events/README.md').read_text()
        blocks = re.findall(r'```sh\n(.*?)```', readme, re.S)
        self.assertEqual(len(blocks), 1)
        with tempfile.TemporaryDirectory() as work:
            outputs = []
            for line in blocks[0].strip().splitlines():
                args = [arg.replace('$TASK_WORK', work) for arg in shlex.split(line)]
                result = subprocess.run(args, cwd=ROOT, capture_output=True, text=True,
                                        timeout=120, env=os.environ | {
                                            'GOPROXY': 'off', 'GOTOOLCHAIN': 'local'})
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                outputs.append(result.stdout)
            self.assertEqual(outputs[0].count('Event:'), 3)
            self.assertIn('Saved next_cursor after 2 events.', outputs[0])
            self.assertIn('Saved next_cursor after 1 events.', outputs[0])
            self.assertIn('Event: correct', outputs[0])
            self.assertIn('Second fact: green.', outputs[0])
            self.assertNotIn('Event:', outputs[1])
            self.assertIn('Second fact: green.', outputs[1])


if __name__ == '__main__':
    unittest.main()
