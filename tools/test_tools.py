#!/usr/bin/env python3
"""仅测试提示词包辅助脚本；不是未来网关的测试。"""
from __future__ import annotations
import json
import shutil
import validate_pack
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

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
            self.assertIn('project.json', text)
            self.assertIn('docs/14-project-identity.md', text)
            self.assertIn('Urbino', text)
            self.assertIn('URBINO_', text)
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
        self.assertEqual(validate_pack.validate_project_identity(ROOT), [])

    def test_every_phase_declares_identity(self):
        phases = json.loads((ROOT / 'phases.json').read_text(encoding='utf-8'))['phases']
        for phase in phases:
            with self.subTest(phase=phase['id']):
                self.assertIn('docs/14-project-identity.md', phase['required_docs'])
                text = (ROOT / phase['prompt']).read_text(encoding='utf-8')
                self.assertIn('project.json', text)
                self.assertIn('docs/14-project-identity.md', text)

    def test_reject_stale_project_state(self):
        with tempfile.TemporaryDirectory() as directory:
            copied = Path(directory) / 'pack'
            shutil.copytree(ROOT, copied)
            state_path = copied / 'progress/state.json'
            state = json.loads(state_path.read_text(encoding='utf-8'))
            state['binary_name'] = 'gateway'
            state['temporary_binary_name'] = 'gateway'
            state_path.write_text(json.dumps(state), encoding='utf-8')
            errors = validate_pack.validate_project_identity(copied)
            self.assertTrue(any('进度项目标识不一致' in error for error in errors))
            self.assertTrue(any('临时二进制字段' in error for error in errors))

    def test_reject_stale_command_reference(self):
        with tempfile.TemporaryDirectory() as directory:
            copied = Path(directory) / 'pack'
            shutil.copytree(ROOT, copied)
            with (copied / 'docs/01-architecture.md').open('a', encoding='utf-8') as handle:
                handle.write('\n错误命令示例：`gateway serve`\n')
            errors = validate_pack.validate_project_identity(copied)
            self.assertTrue(any('旧项目/临时命名残留' in error for error in errors))

    def test_reject_wrong_environment_prefix(self):
        with tempfile.TemporaryDirectory() as directory:
            copied = Path(directory) / 'pack'
            shutil.copytree(ROOT, copied)
            path = copied / 'project.json'
            project = json.loads(path.read_text(encoding='utf-8'))
            project['environment_prefix'] = 'OTHER_'
            path.write_text(json.dumps(project), encoding='utf-8')
            errors = validate_pack.validate_project_identity(copied)
            self.assertTrue(any('environment_prefix' in error for error in errors))

    def test_reject_missing_identity_contract(self):
        with tempfile.TemporaryDirectory() as directory:
            copied = Path(directory) / 'pack'
            shutil.copytree(ROOT, copied)
            path = copied / 'phases.json'
            phases = json.loads(path.read_text(encoding='utf-8'))
            phases['phases'][0]['required_docs'].remove('docs/14-project-identity.md')
            path.write_text(json.dumps(phases), encoding='utf-8')
            errors = validate_pack.validate_project_identity(copied)
            self.assertTrue(any('未引用统一命名规范' in error for error in errors))

if __name__ == '__main__':
    unittest.main(verbosity=2)
