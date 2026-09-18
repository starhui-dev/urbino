#!/usr/bin/env python3
"""离线单元测试：临时目录中使用合成夹具，不调用 OMP 或任何模型。"""
from __future__ import annotations
import copy
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch
import check_omp_runtime
import migration_plan
import validate_omp
import validate_pack

ROOT=Path(__file__).resolve().parents[1]


def example_report(root: Path) -> dict:
    """故意为校验器构造的假数据，只存在临时目录，绝不能作为实机证据。"""
    folder=root/'evidence/synthetic-fixture';folder.mkdir(parents=True,exist_ok=True)
    log=folder/'offline-unit-test.txt';log.write_text('SYNTHETIC UNIT TEST FIXTURE, NOT AN OMP RUN\n')
    data=json.loads((ROOT/'templates/omp-runtime-report.json').read_text())
    data.update(status='pass',checked_at='2026-09-18T00:00:00+00:00',omp_version='synthetic-fixture',
                configuration_fingerprint='a'*64,custom_gateway_only=True,
                main_session_unchanged=True,parent_model_selector='fixture/sol')
    for name,role in data['roles'].items():
        selector='fixture/sol' if name in {'urbino-reviewer','urbino-security'} else 'fixture/worker'
        role.update(requested_selector=selector,resolved_selector=selector,evidence_path=log.relative_to(root).as_posix())
    for check in data['checks'].values():
        check.update(result='pass',evidence_path=log.relative_to(root).as_posix())
    return data


class OmpStructureTests(unittest.TestCase):
    def test_full_structure(self):
        self.assertEqual(validate_omp.validate(ROOT),[])
    def test_native_counts(self):
        self.assertEqual(len(list((ROOT/'.omp/prompts').glob('urbino-*.md'))),10)
        self.assertEqual(len(list((ROOT/'.omp/agents').glob('urbino-*.md'))),5)
        self.assertEqual(int((ROOT/'.omp/skills/urbino-development/SKILL.md').is_file()),1)
    def test_policy_preserves_existing_global_settings(self):
        policy=json.loads((ROOT/'omp/role-policy.json').read_text())
        self.assertIs(policy['mutate_global_config'],False)
        self.assertIs(policy['switch_main_session'],False)
    def test_role_model_binding_can_be_filled_after_setup(self):
        for file in (ROOT/'.omp/agents').glob('urbino-*.md'):
            fm,_,_=validate_omp.frontmatter(file.read_text())
            if 'model' in fm:
                self.assertTrue(fm['model']);self.assertNotIn('<',fm['model']);self.assertNotIn('TODO',fm['model'])
        self.assertTrue(json.loads((ROOT/'omp/role-policy.json').read_text())['native_agents_require_verified_model_binding'])
    def test_duplicate_frontmatter_fails(self):
        with self.assertRaises(ValueError):validate_omp.frontmatter('---\nname: a\nname: b\n---\nx')
    def test_unclosed_frontmatter_fails(self):
        with self.assertRaises(ValueError):validate_omp.frontmatter('---\nname: a')
    def test_no_native_writing_in_reviewer(self):
        for name in ('urbino-reviewer','urbino-security','urbino-scout'):
            fm,_,_=validate_omp.frontmatter((ROOT/f'.omp/agents/{name}.md').read_text())
            tools={x.strip() for x in fm['tools'].split(',')}
            self.assertFalse(tools & {'bash','edit','write','task'})
    def test_invalid_agent_tools_caught(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)/'pack';shutil.copytree(ROOT,root)
            path=root/'.omp/agents/urbino-reviewer.md'
            path.write_text(path.read_text().replace('tools: read, grep, glob','tools: read, grep, glob, bash'))
            self.assertTrue(any('工具边界' in x for x in validate_omp.validate(root)))
    def test_missing_command_target_caught(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)/'pack';shutil.copytree(ROOT,root)
            (root/'prompts/START.md').unlink()
            self.assertTrue(validate_omp.validate(root))
    def test_no_chinese_product_name(self):
        data=json.loads((ROOT/'project.json').read_text())
        self.assertIsNone(data['chinese_name']);self.assertEqual(data['display_name'],'Urbino')
    def test_wrong_product_suffix_caught(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)/'pack';shutil.copytree(ROOT,root)
            path=root/'README.md';path.write_text(path.read_text()+'\nUrbino Gateway\n')
            self.assertTrue(validate_pack.validate_project_identity(root))
    def test_all_new_runtime_checks_not_run(self):
        report=json.loads((ROOT/'templates/omp-runtime-report.json').read_text())
        self.assertEqual(report['status'],'not_run')
        self.assertTrue(all(v['result']=='not_run' for v in report['checks'].values()))


class RuntimeEvidenceTests(unittest.TestCase):
    def test_empty_template_rejected(self):
        report=json.loads((ROOT/'templates/omp-runtime-report.json').read_text())
        self.assertTrue(check_omp_runtime.validate_report(ROOT,report))
    def test_synthetic_fixture_accepts_format_not_authenticity(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);data=example_report(root)
            self.assertEqual(check_omp_runtime.validate_report(root,data),[])
    def test_pass_without_evidence_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);data=example_report(root)
            data['checks']['write_and_test']['evidence_path']='evidence/missing.txt'
            self.assertTrue(check_omp_runtime.validate_report(root,data))
    def test_pass_with_not_run_check_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);data=example_report(root)
            data['checks']['task_schema']['result']='not_run'
            self.assertTrue(check_omp_runtime.validate_report(root,data))
    def test_main_model_changed_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);data=example_report(root);data['main_session_unchanged']=False
            self.assertTrue(check_omp_runtime.validate_report(root,data))
    def test_worker_silent_main_fallback_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);data=example_report(root)
            data['roles']['urbino-implementer']['resolved_selector']=data['parent_model_selector']
            self.assertTrue(check_omp_runtime.validate_report(root,data))
    def test_review_wrong_model_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);data=example_report(root)
            data['roles']['urbino-reviewer']['resolved_selector']='fixture/worker'
            self.assertTrue(check_omp_runtime.validate_report(root,data))
    def test_stale_configuration_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);data=example_report(root)
            self.assertTrue(check_omp_runtime.validate_report(root,data,'b'*64))
    def test_path_escape_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);data=example_report(root)
            data['checks']['cli_version']['evidence_path']='evidence/../../outside.txt'
            self.assertTrue(check_omp_runtime.validate_report(root,data))
    def test_non_evidence_spec_file_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);data=example_report(root);(root/'README.md').write_text('not evidence')
            data['checks']['cli_version']['evidence_path']='README.md'
            self.assertTrue(check_omp_runtime.validate_report(root,data))
    def test_timezone_required(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);data=example_report(root);data['checked_at']='2026-09-18T00:00:00'
            self.assertTrue(check_omp_runtime.validate_report(root,data))
    def test_digest_mismatch_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);data=example_report(root);data['checks']['cli_version']['evidence_sha256']='0'*64
            self.assertTrue(check_omp_runtime.validate_report(root,data))
    def test_blocker_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);data=example_report(root);data['blockers']=['cannot determine route']
            self.assertTrue(check_omp_runtime.validate_report(root,data))
    def test_non_object_rejected(self):
        self.assertTrue(check_omp_runtime.validate_report(ROOT,[]))


class MigrationTests(unittest.TestCase):
    def test_target_unchanged_and_existing_state_preserved(self):
        with tempfile.TemporaryDirectory() as tmp:
            target=Path(tmp)/'target';target.mkdir();(target/'progress').mkdir()
            state=target/'progress/state.json';state.write_text('{"actual_progress":true}')
            agent=target/'AGENTS.md';agent.write_text('existing custom rules')
            before={p.relative_to(target).as_posix():p.read_bytes() for p in target.rglob('*') if p.is_file()}
            report=migration_plan.build_plan(ROOT,target)
            after={p.relative_to(target).as_posix():p.read_bytes() for p in target.rglob('*') if p.is_file()}
            self.assertEqual(before,after);self.assertFalse(report['target_mutated'])
            actions={r['path']:r['action'] for r in report['files']}
            self.assertEqual(actions['progress/state.json'],'PRESERVE_RUNTIME')
            self.assertEqual(actions['AGENTS.md'],'MERGE_REQUIRED')
            self.assertEqual(actions['checklists/test-matrix.csv'],'PRESERVE_RUNTIME')
    def test_identical_file_identified(self):
        with tempfile.TemporaryDirectory() as tmp:
            target=Path(tmp);shutil.copyfile(ROOT/'AGENTS.md',target/'AGENTS.md')
            row=next(r for r in migration_plan.build_plan(ROOT,target)['files'] if r['path']=='AGENTS.md')
            self.assertEqual(row['action'],'IDENTICAL')
    def test_same_and_nested_paths_rejected(self):
        with self.assertRaises(ValueError):migration_plan.build_plan(ROOT,ROOT)
        with self.assertRaises(ValueError):migration_plan.build_plan(ROOT,ROOT/'tools')
    def test_missing_target_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaises(ValueError):migration_plan.build_plan(ROOT,Path(tmp)/'missing')
    def test_sensitive_content_not_read(self):
        with tempfile.TemporaryDirectory() as tmp:
            source=Path(tmp)/'source';target=Path(tmp)/'target';source.mkdir();target.mkdir()
            (source/'.env').write_text('synthetic secret');(target/'.env').write_text('different secret')
            with patch.object(Path,'read_bytes',side_effect=AssertionError('should not read secret')):
                report=migration_plan.build_plan(source,target)
            self.assertEqual(report['files'][0]['action'],'PRESERVE_RUNTIME')
            self.assertNotIn('synthetic secret',json.dumps(report))
    def test_symbolic_link_target_blocked(self):
        with tempfile.TemporaryDirectory() as tmp:
            target=Path(tmp)/'target';target.mkdir();outside=Path(tmp)/'outside.txt';outside.write_text('outside')
            try:(target/'AGENTS.md').symlink_to(outside)
            except (OSError,NotImplementedError):self.skipTest('symlinks unsupported on this platform')
            row=next(r for r in migration_plan.build_plan(ROOT,target)['files'] if r['path']=='AGENTS.md')
            self.assertEqual(row['action'],'BLOCKED_PATH')
    def test_cli_wont_write_report_into_target(self):
        with tempfile.TemporaryDirectory() as tmp:
            target=Path(tmp)
            result=subprocess.run([sys.executable,str(ROOT/'tools/migration_plan.py'),'--target',str(target),'--out',str(target/'plan.json')],capture_output=True,text=True,timeout=10)
            self.assertNotEqual(result.returncode,0);self.assertFalse((target/'plan.json').exists())
    def test_cli_refuses_report_overwrite(self):
        with tempfile.TemporaryDirectory() as tmp:
            target=Path(tmp)/'target';target.mkdir();out=Path(tmp)/'plan.json';out.write_text('keep')
            result=subprocess.run([sys.executable,str(ROOT/'tools/migration_plan.py'),'--target',str(target),'--out',str(out)],capture_output=True,text=True,timeout=10)
            self.assertNotEqual(result.returncode,0);self.assertEqual(out.read_text(),'keep')

if __name__=='__main__':unittest.main(verbosity=2)
