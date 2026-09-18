#!/usr/bin/env python3
"""只读生成提示词包的迁移差异计划，不复制/删除/覆盖目标文件，不读取秘密文件。"""
from __future__ import annotations
import argparse
import hashlib
import json
from pathlib import Path
import sys

ROOT=Path(__file__).resolve().parents[1]
EXCLUDE_PARTS={'.git','__pycache__','.pytest_cache','.ruff_cache'}
ARCHIVE_ONLY={'MANIFEST.sha256','PACK_VALIDATION.md'}


def protected(rel: str) -> bool:
    path=Path(rel)
    return (path.parts[0] in {'progress','evidence'} or rel in {'checklists/test-matrix.csv','checklists/omp-readiness.csv'}
            or path.name in {'.env','auth.json','auth.yml','auth.yaml','config.yml','config.yaml','settings.json','models.yml','models.yaml','models.json','config.json','settings.yml','settings.yaml','credentials.json','urbino.yaml'}
            or path.name.startswith('.env.') or path.suffix.lower() in {'.pem','.key','.p12','.pfx'})


def build_plan(source: Path, target: Path) -> dict:
    if source.is_symlink() or target.is_symlink():raise ValueError('源/目标根目录不能是符号链接')
    source=source.resolve();target=target.resolve()
    if not source.is_dir() or not target.is_dir():raise ValueError('源和目标必须是现有目录')
    if source==target or source.is_relative_to(target) or target.is_relative_to(source):
        raise ValueError('新版包和已有项目必须是互不嵌套的不同目录')
    rows=[]
    for incoming in sorted(source.rglob('*')):
        rel=incoming.relative_to(source)
        if any(part in EXCLUDE_PARTS for part in rel.parts) or incoming.suffix=='.pyc':continue
        if incoming.is_symlink():raise ValueError(f'发行源含不支持的符号链接: {rel}')
        if not incoming.is_file():continue
        name=rel.as_posix();dest=target/rel
        if name in ARCHIVE_ONLY:
            rows.append({'path':name,'action':'ARCHIVE_ONLY','reason':'仅描述未修改发行包，不替换工作仓库证据'});continue
        # 保全文件只记录路径，不读取内容/摘要。存在性不足以代表权限或可覆盖性。
        if protected(name):
            rows.append({'path':name,'action':'PRESERVE_RUNTIME','reason':'已有运行数据/秘密不可覆盖；缺失时按实际情况初始化或合并'});continue
        if not dest.resolve().is_relative_to(target) or dest.is_symlink() or any((target/Path(*rel.parts[:i])).is_symlink() for i in range(1,len(rel.parts))):
            rows.append({'path':name,'action':'BLOCKED_PATH','reason':'目标路径经过符号链接或越界'});continue
        src_hash=hashlib.sha256(incoming.read_bytes()).hexdigest()
        item={'path':name,'source_sha256':src_hash}
        if not dest.exists():item['action']='ADD_CANDIDATE'
        elif not dest.is_file():item.update(action='BLOCKED_PATH',reason='目标同名项不是普通文件')
        else:
            dst_hash=hashlib.sha256(dest.read_bytes()).hexdigest();item['target_sha256']=dst_hash
            item['action']='IDENTICAL' if dst_hash==src_hash else 'MERGE_REQUIRED'
        rows.append(item)
    counts={}
    for row in rows:counts[row['action']]=counts.get(row['action'],0)+1
    return {'schema_version':1,'mode':'read_only_plan','source':str(source),'target':str(target),
            'target_mutated':False,'target_only_files':'preserve; not scanned or deleted',
            'warning':'此报告不是执行授权；先备份与审阅再逐项合并。未判断修改对已验收阶段的影响。',
            'counts':counts,'files':rows}


def main() -> int:
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--target',type=Path,required=True)
    parser.add_argument('--out',type=Path,help='独占创建一个仓库之外的计划文件，已有文件不覆盖')
    args=parser.parse_args()
    try:
        plan=build_plan(ROOT,args.target)
        payload=json.dumps(plan,ensure_ascii=False,indent=2)+'\n'
        if args.out:
            out=args.out.expanduser().resolve()
            if out.is_relative_to(args.target.resolve()) or out.is_relative_to(ROOT):
                raise ValueError('计划文件必须放在源包和目标仓库外')
            with out.open('x',encoding='utf-8',newline='\n') as handle:handle.write(payload)
            print(f'已写只读差异计划: {out}；目标仓库未修改。')
        else:sys.stdout.write(payload)
        return 0
    except (OSError,ValueError) as exc:
        print(f'ERROR: 迁移计划失败: {exc}',file=sys.stderr);return 2

if __name__=='__main__':
    raise SystemExit(main())
