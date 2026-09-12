#!/usr/bin/env python3
"""核对阶段证据格式和本地引用；不证明命令确实执行，不替代独立复核。"""
from __future__ import annotations
import argparse
import csv
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

def repo_file(relative: str) -> Path:
    path = (ROOT / relative).resolve()
    if not path.is_relative_to(ROOT):
        raise ValueError(f"证据文件必须位于仓库内：{relative}")
    return path

def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("stage", help="00至19")
    parser.add_argument("--file", help="仓库相对路径；默认evidence/stages/NN.json")
    parser.add_argument("--require-review", action="store_true")
    parser.add_argument("--expected-revision", help="调用者从实际代码计算的revision/tree hash")
    args = parser.parse_args()
    errors: list[str] = []
    try:
        if not args.stage.isdigit() or not 0 <= int(args.stage) <= 19:
            parser.error("stage应为00至19")
        stage = f"{int(args.stage):02d}"
        if args.require_review and not args.expected_revision:
            parser.error("--require-review须同时指定从实际代码核对的--expected-revision")
        path = repo_file(args.file or f"evidence/stages/{stage}.json")
        data = json.loads(path.read_text(encoding="utf-8"))
        if data.get("stage") != stage:
            errors.append("证据阶段不匹配")
        if data.get("status") not in {"implemented", "verified"}:
            errors.append("阶段不是implemented/verified")
        revision = data.get("code_revision")
        if not isinstance(revision, str) or not revision.strip():
            errors.append("缺实际code_revision")
        if args.expected_revision and revision != args.expected_revision:
            errors.append("代码revision与实际待检查版本不一致")
        if data.get("blockers"):
            errors.append("仍存在blocker")
        commands = data.get("commands", [])
        if not commands:
            errors.append("没有命令证据")
        for index, command in enumerate(commands):
            if not command.get("required", True):
                continue
            label = f"command[{index}]"
            if not command.get("command") or not command.get("started_at") or not command.get("finished_at"):
                errors.append(f"{label}缺实际命令或执行时间")
            if command.get("result") != "pass" or command.get("exit_code") != 0:
                errors.append(f"{label}未实际通过")
            if command.get("tests_failed") not in (None, 0) or command.get("tests_skipped") not in (None, 0):
                errors.append(f"{label}存在失败或跳过")
            output = command.get("output_path")
            if not output or not repo_file(output).is_file() or repo_file(output).stat().st_size == 0:
                errors.append(f"{label}缺非空输出文件")
        with repo_file("checklists/test-matrix.csv").open(encoding="utf-8-sig", newline="") as handle:
            expected = {r["test_id"] for r in csv.DictReader(handle) if r["phase"] == stage and r["required"] == "true"}
        requirements = data.get("requirements", [])
        actual = [r.get("test_id") for r in requirements]
        if len(actual) != len(set(actual)):
            errors.append("重复的需求证据编号")
        if not expected.issubset(set(actual)):
            errors.append("缺必需测试映射：" + ", ".join(sorted(expected - set(actual))))
        for item in requirements:
            if item.get("test_id") not in expected:
                continue
            if item.get("result") != "pass" or not item.get("test_name"):
                errors.append(f'{item.get("test_id")}没有实际通过的测试')
            for field in ("source_path", "evidence_path"):
                ref = item.get(field)
                if not ref or not repo_file(ref).is_file():
                    errors.append(f'{item.get("test_id")}缺{field}')
        if args.require_review:
            review = data.get("review", {})
            if data.get("status") != "verified" or review.get("status") != "pass":
                errors.append("尚未独立复核通过")
            if review.get("reviewed_revision") != revision:
                errors.append("复核的代码版本不一致")
            if not review.get("reviewer_session") or not review.get("reviewed_at"):
                errors.append("缺独立会话/时间记录")
            report = review.get("report_path")
            if not report or not repo_file(report).is_file():
                errors.append("缺复核报告")
        if errors:
            for error in errors:
                print(f"ERROR: {error}", file=sys.stderr)
            return 1
        print(f"阶段{stage}证据结构与本地文件引用通过。仍需独立检查输出真实性与测试覆盖。")
        return 0
    except (OSError, ValueError, KeyError, TypeError, json.JSONDecodeError) as exc:
        print(f"证据检查失败：{exc}", file=sys.stderr)
        return 2

if __name__ == "__main__":
    raise SystemExit(main())
