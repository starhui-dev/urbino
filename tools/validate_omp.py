#!/usr/bin/env python3
"""离线检查本包 OMP 文件约定；不是 OMP 加载器或运行验证。仅标准库。"""
from __future__ import annotations
import argparse
import csv
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
COMMANDS = {
    'urbino-setup': 'OMP_SETUP.md', 'urbino-start': 'START.md',
    'urbino-next': 'NEXT.md', 'urbino-phase': 'NEXT.md',
    'urbino-review': 'REVIEW.md', 'urbino-resume': 'RESUME.md',
    'urbino-repair': 'REPAIR.md', 'urbino-status': 'STATUS.md',
    'urbino-release': 'RELEASE.md', 'urbino-migrate': 'MIGRATE_TO_OMP.md',
}
ROLES = {'urbino-scout': False, 'urbino-implementer': True, 'urbino-tester': True,
         'urbino-reviewer': False, 'urbino-security': False}
CHECKS = ('cli_version', 'prompt_discovery', 'agent_discovery', 'skill_discovery',
          'task_schema', 'role_resolution', 'readonly_scout', 'write_and_test',
          'independent_review', 'all_tasks_terminal', 'configuration_preserved')


def frontmatter(text: str) -> tuple[dict[str, str], str, str]:
    """只提取本包简单单行顶层字段；明确不实现/替代通用 YAML 解析。"""
    lines = text.splitlines()
    if not lines or lines[0] != '---':
        raise ValueError('缺少 frontmatter 起始分隔符')
    try:
        end = lines.index('---', 1)
    except ValueError as exc:
        raise ValueError('缺少 frontmatter 结束分隔符') from exc
    raw = '\n'.join(lines[1:end])
    result: dict[str, str] = {}
    for line in lines[1:end]:
        match = re.match(r'^([A-Za-z][\w-]*):\s*(.*)$', line)
        if match:
            key, value = match.groups()
            if key in result:
                raise ValueError(f'重复字段 {key}')
            result[key] = value.strip().strip('"\'')
    return result, raw, '\n'.join(lines[end+1:])


def validate(root: Path) -> list[str]:
    errors: list[str] = []
    root = root.resolve()
    def text(rel: str) -> str:
        path = root / rel
        if path.is_symlink() or not path.resolve().is_relative_to(root):
            raise ValueError(f'不支持的资源路径 {rel}')
        return path.read_text(encoding='utf-8')
    try:
        policy = json.loads(text('omp/role-policy.json'))
        if policy.get('kind') != 'urbino-workflow-policy-not-omp-config':
            errors.append('role-policy 不得冒充 OMP settings')
        for key, wanted in {'use_existing_custom_gateway_only':True, 'mutate_global_config':False,
                            'switch_main_session':False, 'initial_concurrency':2,
                            'max_concurrency':4, 'max_child_depth':1,
                            'native_agents_require_verified_model_binding':True,
                            'state_writer':'main-session', 'review_context':'fresh'}.items():
            if policy.get(key) != wanted:
                errors.append(f'工作流策略不一致: {key}')
        if set(policy.get('roles', {})) != set(ROLES):
            errors.append('五类角色定义不完整')
        actual = {p.stem for p in (root / '.omp/prompts').glob('urbino-*.md')}
        if actual != set(COMMANDS):
            errors.append('项目提示词入口不完整或有未知 urbino-* 入口')
        for command, target in COMMANDS.items():
            fm, _, body = frontmatter(text(f'.omp/prompts/{command}.md'))
            if not fm.get('description') or f'prompts/{target}' not in body:
                errors.append(f'{command}: description/目标不完整')
            if '$ARGUMENTS' not in body or not (root / 'prompts' / target).is_file():
                errors.append(f'{command}: 参数入口/目标不存在')
        for name, writes in ROLES.items():
            fm, raw, body = frontmatter(text(f'.omp/agents/{name}.md'))
            if fm.get('name') != name or not fm.get('description'):
                errors.append(f'{name}: name/description 不完整')
            # 本发行包明确使用 CSV；不是任意版本全部 YAML 语法验证。
            tools = {t.strip() for t in fm.get('tools', '').split(',') if t.strip()}
            expected = {'read','grep','glob'} | ({'write','edit','bash'} if writes else set())
            if tools - {'yield'} != expected:
                errors.append(f'{name}: 工具边界改变')
            if 'task' in tools or 'spawns' in fm:
                errors.append(f'{name}: 不允许递归分派')
            if not body.strip() or 'progress/state.json' not in body:
                errors.append(f'{name}: 状态写入边界缺失')
            if 'output' not in fm or '  properties:' not in raw:
                errors.append(f'{name}: 缺少结构化输出')
            for field in ('status','summary','changed_paths','evidence_paths','findings','blockers'):
                if f'    {field}:' not in raw:
                    errors.append(f'{name}: 输出缺少 {field}')
            for field in ('prewalk', 'advisor'):
                if field in fm and fm[field] != 'false':
                    errors.append(f'{name}: 隐含调用策略改变 {field}')
            # model 缺省是发行模板的有意设计，实机门禁负责验证绑定；此处不声称已绑定。
            if 'model' in fm and (not fm['model'] or 'TODO' in fm['model'] or '<' in fm['model']):
                errors.append(f'{name}: model 存在占位值')
            info = policy['roles'].get(name, {})
            if info.get('native_file') != f'.omp/agents/{name}.md' or info.get('write_access') is not writes:
                errors.append(f'{name}: 项目策略与文件不一致')
        fm, _, body = frontmatter(text('.omp/skills/urbino-development/SKILL.md'))
        if fm.get('name') != 'urbino-development' or not fm.get('description') or not body:
            errors.append('skill 名称/描述/正文不完整')
        phases = json.loads(text('phases.json'))['phases']
        for phase in phases:
            if 'docs/15-omp-workflow.md' not in phase['required_docs']:
                errors.append(f"{phase['id']}: 未声明 OMP 工作流")
            if phase.get('orchestrator') != 'main-session' or phase.get('single_state_writer') is not True:
                errors.append(f"{phase['id']}: 缺少主会话状态边界")
        with (root / 'checklists/omp-readiness.csv').open(encoding='utf-8', newline='') as handle:
            rows = list(csv.DictReader(handle))
        if len(rows) != 24 or len({r['check_id'] for r in rows}) != 24:
            errors.append('OMP 准备度清单应有 24 个唯一条目')
        for row in rows:
            if not row['scenario'] or not row['expected_result'] or row['required'] != 'true':
                errors.append('OMP 准备度条目字段不完整')
        report = json.loads(text('templates/omp-runtime-report.json'))
        if set(report.get('roles',{})) != set(ROLES) or set(report.get('checks',{})) != set(CHECKS):
            errors.append('运行报告模板缺少角色或必测项')
        if report.get('status') != 'not_run':
            errors.append('发行模板不能冒充实际运行报告')
        project = json.loads(text('project.json'))
        if project.get('development_harness') != 'oh-my-pi' or project.get('development_command') != 'omp':
            errors.append('项目开发工具契约不一致')
        for rel in ('docs/16-omp-model-routing.md','docs/17-omp-migration-recovery.md',
                    'references/OMP-SOURCES.md','references/omp-sources.json',
                    'templates/OMP_TASK.md','templates/OMP_HANDOFF.md',
                    'tools/check_omp_runtime.py','tools/migration_plan.py'):
            if not text(rel).strip():
                errors.append(f'空文件 {rel}')
    except (OSError, ValueError, KeyError, TypeError) as exc:
        errors.append(f'OMP 结构无法校验: {exc}')
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, default=ROOT, help='只读检查此包目录')
    args = parser.parse_args()
    errors = validate(args.root)
    for error in errors:
        print(f'ERROR: {error}', file=sys.stderr)
    if errors:
        return 1
    print('OMP 文件约定通过：10 个入口、5 个 Agent、1 个 skill、20 阶段、24 条准备度要求。')
    print('未运行 OMP、未调用模型；本工具不是 YAML/OMP 兼容性认证或权限沙箱。')
    return 0

if __name__ == '__main__':
    raise SystemExit(main())
