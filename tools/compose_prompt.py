#!/usr/bin/env python3
"""离线组装一个阶段的提示词；不调用 OMP、不执行开发命令。"""
from __future__ import annotations
import argparse
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

def read_repo_file(relative: str) -> str:
    path = (ROOT / relative).resolve()
    if not path.is_relative_to(ROOT):
        raise ValueError(f"文件路径越界：{relative}")
    return path.read_text(encoding="utf-8")

def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("phase", nargs="?", help="阶段编号，00 至 19")
    parser.add_argument("--list", action="store_true", help="列出阶段")
    parser.add_argument("--out", type=Path, help="写入新文件，已有文件不会被覆盖")
    args = parser.parse_args()
    try:
        phases = json.loads(read_repo_file("phases.json"))["phases"]
        if args.list:
            for phase in phases:
                print(f'{phase["id"]}  {phase["title"]}')
            return 0
        if args.phase is None or not args.phase.isdigit():
            parser.error("请指定 00 至 19 的阶段，或使用 --list")
        phase_id = f"{int(args.phase):02d}"
        phase = next((p for p in phases if p["id"] == phase_id), None)
        if phase is None:
            parser.error(f"未知阶段：{phase_id}")
        files = list(dict.fromkeys([
            "AGENTS.md", "MASTER_PROMPT.md", "project.json", "omp/role-policy.json", "progress/state.json",
            phase["prompt"], *phase["required_docs"],
        ]))
        parts = [
            f'# Urbino · OMP 阶段 {phase_id} 组合提示词\n\n'
            '在本仓库中仅执行指定阶段。以下为仓库规格的离线副本；执行前仍需检查实际源码、进度和前置门禁。\n'
            '本文件不会启动 OMP，也不是已完成的开发结果。'
        ]
        for relative in files:
            parts.append(f"\n---\n\n# 仓库文件：{relative}\n\n{read_repo_file(relative).strip()}")
        result = "\n".join(parts) + "\n"
        if args.out:
            destination = args.out.expanduser().resolve()
            if not destination.parent.is_dir():
                raise ValueError(f"输出目录不存在：{destination.parent}")
            # 独占创建，避免覆盖仓库规范、密钥或已有输出。
            with destination.open("x", encoding="utf-8", newline="\n") as handle:
                handle.write(result)
            print(f"已生成阶段提示词：{destination}")
        else:
            sys.stdout.write(result)
        return 0
    except (OSError, ValueError, KeyError, json.JSONDecodeError) as exc:
        print(f"失败：{exc}", file=sys.stderr)
        return 2

if __name__ == "__main__":
    raise SystemExit(main())
