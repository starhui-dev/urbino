#!/usr/bin/env python3
"""仅测试提示词包辅助脚本；不是未来网关的测试。"""
from __future__ import annotations
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from validate_pack import PROJECT_IDENTITY, validate_project_identity

ROOT = Path(__file__).resolve().parents[1]

def run_tool(name: str, *args: str) -> subprocess.CompletedProcess:
    return subprocess.run([sys.executable, str(ROOT / 'tools' / name), *args],
                          cwd=ROOT, text=True, capture_output=True, encoding='utf-8', timeout=10)

class ToolTests(unittest.TestCase):
    def test_pack_structure(self):
        result = run_tool('validate_pack.py')
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_phase_list(self):
        result = run_tool('compose_prompt.py', '--list')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(result.stdout.strip().splitlines()), 20)

    def test_compose_and_refuse_overwrite(self):
        with tempfile.TemporaryDirectory() as directory:
            dest = Path(directory) / 'prompt.md'
            result = run_tool('compose_prompt.py', '00', '--out', str(dest))
            self.assertEqual(result.returncode, 0, result.stderr)
            text = dest.read_text(encoding='utf-8')
            self.assertIn('prompts/00-foundation.md', text)
            self.assertIn('docs/00-scope.md', text)
            retry = run_tool('compose_prompt.py', '00', '--out', str(dest))
            self.assertNotEqual(retry.returncode, 0)
            self.assertEqual(dest.read_text(encoding='utf-8'), text)

    def test_invalid_phase(self):
        self.assertNotEqual(run_tool('compose_prompt.py', '99').returncode, 0)

    def test_template_is_not_pass(self):
        result = run_tool('check_evidence.py', '00', '--file', 'templates/stage-evidence.json')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('ERROR', result.stderr)

    def test_review_requires_current_revision(self):
        result = run_tool('check_evidence.py', '00', '--file', 'templates/stage-evidence.json', '--require-review')
        self.assertNotEqual(result.returncode, 0)

    def test_project_identity(self):
        state = json.loads((ROOT / 'progress/state.json').read_text(encoding='utf-8'))
        self.assertEqual(validate_project_identity(state), [])
        self.assertEqual(state['project_name'], 'Urbino')

    def test_identity_rejects_drift(self):
        for field in PROJECT_IDENTITY:
            with self.subTest(field=field):
                changed = dict(PROJECT_IDENTITY)
                changed[field] = 'incorrect'
                self.assertTrue(validate_project_identity(changed))
        changed = dict(PROJECT_IDENTITY, temporary_binary_name='legacy')
        self.assertTrue(validate_project_identity(changed))
        changed = dict(PROJECT_IDENTITY, project_name_zh='disallowed')
        self.assertTrue(validate_project_identity(changed))

    def test_config_and_egress_names(self):
        config = (ROOT / 'docs/10-config-operations.md').read_text(encoding='utf-8')
        for name in ('urbino.yaml', 'URBINO_CONFIG', 'configs/urbino.example.yaml',
                     'configs/urbino.production.example.yaml', 'URBINO_'):
            self.assertIn(name, config)
        transport = (ROOT / 'docs/05-transport-egress.md').read_text(encoding='utf-8')
        self.assertIn('urbino/<version>', transport)

    def test_all_phases_include_fixed_name(self):
        for index in range(20):
            with self.subTest(phase=index):
                result = run_tool('compose_prompt.py', f'{index:02d}')
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn(f'# Urbino · Codex 阶段 {index:02d}', result.stdout)
                self.assertIn('项目正式名称固定为 `Urbino`，不设中文名', result.stdout)
                self.assertIn('URBINO_', result.stdout)

if __name__ == '__main__':
    unittest.main(verbosity=2)
