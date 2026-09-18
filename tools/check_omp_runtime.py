#!/usr/bin/env python3
"""只读校验 OMP 运行证据的完备性；无法替代人工/主 Agent 对日志真实性的审查。"""
from __future__ import annotations
import argparse
from datetime import datetime
import hashlib
import json
from pathlib import Path
import re
import sys
from validate_omp import ROLES, CHECKS

ROOT = Path(__file__).resolve().parents[1]


def validate_report(root: Path, data: dict, expected_fingerprint: str | None = None) -> list[str]:
    errors: list[str] = []
    root = root.resolve()
    def nonempty(value: object) -> bool:
        return isinstance(value,str) and bool(value.strip()) and value.strip().lower() not in {'unknown','todo','not_run','null','placeholder'} and '<' not in value
    def evidence(item: object, where: str) -> None:
        if not isinstance(item,dict):
            errors.append(f'{where}: 记录必须是对象');return
        rel=item.get('evidence_path')
        if not nonempty(rel):
            errors.append(f'{where}: 缺少实际 evidence_path');return
        path=root / rel
        if Path(rel).is_absolute() or not rel.replace('\\','/').startswith('evidence/') or not path.resolve().is_relative_to(root / 'evidence') or path.is_symlink():
            errors.append(f'{where}: 证据必须是仓库 evidence/ 内的普通文件');return
        if not path.is_file() or path.stat().st_size==0:
            errors.append(f'{where}: 证据文件不存在或为空');return
        claimed=item.get('evidence_sha256')
        if claimed is not None and (not re.fullmatch(r'[0-9a-f]{64}',str(claimed)) or hashlib.sha256(path.read_bytes()).hexdigest()!=claimed):
            errors.append(f'{where}: 证据摘要不匹配')
    if not isinstance(data,dict):
        return ['运行报告必须是对象']
    for key,wanted in {'schema_version':1,'harness':'oh-my-pi','status':'pass',
                       'configuration_fingerprint_includes_secrets':False,
                       'custom_gateway_only':True,'main_session_unchanged':True}.items():
        if data.get(key)!=wanted or (isinstance(wanted,bool) and data.get(key) is not wanted):
            errors.append(f'{key} 未满足 {wanted!r}')
    for key in ('omp_version','parent_model_selector'):
        if not nonempty(data.get(key)):
            errors.append(f'{key} 缺少真实值')
    try:
        timestamp=datetime.fromisoformat(str(data.get('checked_at')).replace('Z','+00:00'))
        if timestamp.tzinfo is None:raise ValueError('missing timezone')
    except ValueError:
        errors.append('checked_at 必须是含时区的实际 ISO 时间')
    fp=data.get('configuration_fingerprint')
    if not isinstance(fp,str) or not re.fullmatch('[0-9a-f]{64}',fp):
        errors.append('缺少已脱敏配置元数据的 SHA-256 指纹')
    if expected_fingerprint is not None and fp!=expected_fingerprint:
        errors.append('报告不是当前配置指纹；应重新验证')
    if data.get('blockers')!=[]:
        errors.append('仍存在阻塞或缺少 blockers 列表')
    roles=data.get('roles',{})
    if not isinstance(roles,dict):return errors+['roles 必须是对象']
    for name in ROLES:
        role=roles.get(name)
        evidence(role,f'角色 {name}')
        if not isinstance(role,dict):continue
        for field in ('requested_selector','resolved_selector'):
            if not nonempty(role.get(field)):
                errors.append(f'{name}: {field} 未验证')
        resolved=role.get('resolved_selector')
        if name in {'urbino-reviewer','urbino-security'} and resolved!=data.get('parent_model_selector'):
            errors.append(f'{name}: 审查应使用已验证的主 Sol 模型；别名须归一化')
        if name in {'urbino-scout','urbino-implementer','urbino-tester'} and resolved==data.get('parent_model_selector'):
            errors.append(f'{name}: 不得静默回退为主模型；需要实际低成本路由')
    checks=data.get('checks',{})
    if not isinstance(checks,dict):return errors+['checks 必须是对象']
    for key in CHECKS:
        item=checks.get(key)
        evidence(item,f'检查 {key}')
        if not isinstance(item,dict) or item.get('result')!='pass':
            errors.append(f'{key}: 未实际通过')
    return errors


def main() -> int:
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--file', type=Path, default=Path('evidence/omp/runtime.json'))
    parser.add_argument('--expected-fingerprint', help='当前已脱敏配置的 SHA-256；不传时由主 Agent 对比')
    args=parser.parse_args()
    try:
        path=args.file if args.file.is_absolute() else ROOT/args.file
        if not path.resolve().is_relative_to(ROOT):raise ValueError('报告路径越界')
        errors=validate_report(ROOT,json.loads(path.read_text(encoding='utf-8')),args.expected_fingerprint)
        for error in errors:print(f'ERROR: {error}',file=sys.stderr)
        if errors:return 1
        print('运行记录字段与证据文件检查通过；仍须审阅日志、实际路由和当前配置的一致性。')
        print('本检查器没有执行 OMP/模型，也不表示 Urbino 产品或上线验收通过。')
        return 0
    except (OSError,ValueError,TypeError) as exc:
        print(f'ERROR: 无法校验运行报告: {exc}',file=sys.stderr);return 2

if __name__=='__main__':
    raise SystemExit(main())
