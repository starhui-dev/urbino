#!/usr/bin/env python3
"""检查提示词包结构；不会编译、测试或认证未来网关。"""
from __future__ import annotations
import argparse
import csv
import hashlib
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

PROJECT_IDENTITY = {
    "project_name": "Urbino",
    "project_slug": "urbino",
    "binary_name": "urbino",
    "config_filename": "urbino.yaml",
    "env_prefix": "URBINO_",
}

def validate_project_identity(state: dict) -> list[str]:
    """Validate immutable naming metadata, not development completion."""
    errors: list[str] = []
    for field, expected in PROJECT_IDENTITY.items():
        if state.get(field) != expected:
            errors.append(f"项目命名不一致：{field} 应为 {expected}")
    if "temporary_binary_name" in state:
        errors.append("项目名称已确定，不能保留临时二进制字段")
    if any(state.get(field) for field in ("chinese_name", "project_name_zh")):
        errors.append("Urbino 不设中文名")
    return errors

def safe_path(relative: str) -> Path:
    path = (ROOT / relative).resolve()
    if not path.is_relative_to(ROOT):
        raise ValueError(f"越界路径：{relative}")
    return path

def load_json(relative: str):
    return json.loads(safe_path(relative).read_text(encoding="utf-8"))

def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--checksums", action="store_true", help="同时校验发行包 MANIFEST.sha256；开发后文件改动会导致不匹配")
    args = parser.parse_args()
    errors: list[str] = []
    try:
        required = ["README.md", "AGENTS.md", "MASTER_PROMPT.md", "phases.json",
                    "progress/state.json", "references/SOURCES.md", "references/sources.json",
                    "checklists/test-matrix.csv", "checklists/RELEASE_GATES.md",
                    "templates/stage-evidence.json", "prompts/REVIEW.md", "prompts/RESUME.md",
                    "prompts/REPAIR.md", "tools/compose_prompt.py", "tools/check_evidence.py"]
        for relative in required:
            path = safe_path(relative)
            if not path.is_file() or path.stat().st_size == 0:
                errors.append(f"缺失或空文件：{relative}")
        phases = load_json("phases.json")["phases"]
        ids = [p["id"] for p in phases]
        if ids != [f"{i:02d}" for i in range(20)]:
            errors.append("阶段编号应连续为00至19且不能重复")
        seen: set[str] = set()
        for phase in phases:
            for dep in phase["depends_on"]:
                if dep not in seen:
                    errors.append(f'阶段{phase["id"]}依赖未定义或后置阶段{dep}')
            for relative in [phase["prompt"], *phase["required_docs"]]:
                path = safe_path(relative)
                if not path.is_file() or path.stat().st_size == 0:
                    errors.append(f'阶段{phase["id"]}引用不存在：{relative}')
            seen.add(phase["id"])
        progress = load_json("progress/state.json")
        errors.extend(validate_project_identity(progress))
        if load_json("phases.json").get("project_name") != "Urbino":
            errors.append("phases.json 项目名称应为 Urbino")
        states = progress["stages"]
        valid_states = {"not_started", "in_progress", "implemented", "verified", "blocked"}
        if set(states) != set(ids):
            errors.append("进度阶段与phases.json不一致")
        for key, state in states.items():
            if state.get("status") not in valid_states:
                errors.append(f"无效阶段状态：{key}")
        with safe_path("checklists/test-matrix.csv").open(encoding="utf-8-sig", newline="") as handle:
            rows = list(csv.DictReader(handle))
        test_ids = [r["test_id"] for r in rows]
        if len(test_ids) != len(set(test_ids)):
            errors.append("测试编号重复")
        if {r["phase"] for r in rows} != set(ids):
            errors.append("存在无测试阶段或测试引用无效阶段")
        for row in rows:
            if not row["scenario"] or not row["expected_result"]:
                errors.append(f'测试缺说明：{row["test_id"]}')
        sources = load_json("references/sources.json")["sources"]
        if len({s["id"] for s in sources}) != len(sources):
            errors.append("来源编号重复")
        for source in sources:
            if not source["url"].startswith("https://"):
                errors.append(f'来源URL格式不正确：{source["id"]}')
        if safe_path("AGENTS.md").stat().st_size >= 32768:
            errors.append("根AGENTS.md过大")
        naming_contracts = {
            "README.md": ("Urbino", "urbino.yaml", "URBINO_"),
            "AGENTS.md": ("Urbino", "cmd/urbino/", "URBINO_"),
            "MASTER_PROMPT.md": ("Urbino", "不设中文名"),
            "docs/00-scope.md": ("Urbino", "urbino.yaml", "URBINO_", "urbino-worker"),
            "docs/01-architecture.md": ("cmd/urbino/", "urbino serve", "urbino worker"),
            "docs/05-transport-egress.md": ("urbino/<version>",),
            "docs/10-config-operations.md": ("configs/urbino.example.yaml", "configs/urbino.production.example.yaml", "URBINO_"),
            "prompts/00-foundation.md": ("cmd/urbino", "example.com/urbino"),
            "prompts/17-deployment.md": ("urbino-worker", "configs/urbino.example.yaml"),
        }
        for relative, expected_tokens in naming_contracts.items():
            content = safe_path(relative).read_text(encoding="utf-8")
            for token in expected_tokens:
                if token not in content:
                    errors.append(f"命名契约缺失：{relative} -> {token}")
        for path in safe_path("prompts").glob("*.md"):
            content = path.read_text(encoding="utf-8")
            if "项目正式名称固定为 `Urbino`，不设中文名" not in content:
                errors.append(f"提示词缺少固定命名：{path.relative_to(ROOT)}")
        legacy_identifier = re.compile(
            r"\bcmd/gateway\b|example\.com/ai-gateway|\bGATEWAY_[A-Z_]*"
            r"|\bgateway(?:\s+(?:serve|worker|migrate|bootstrap|version|admin|doctor|health|config|ledger)\b|/<version>)"
            r"|configs/config(?:\.production)?\.example\.yaml|\bUrbino\s+Gateway\b"
        )
        for path in ROOT.rglob("*.md"):
            content = path.read_text(encoding="utf-8")
            if legacy_identifier.search(content):
                errors.append(f"遗留或不一致的项目标识：{path.relative_to(ROOT)}")
            if "\x00" in content:
                errors.append(f"文本含NUL：{path.relative_to(ROOT)}")
            # 只检查真正的Markdown本地链接，不检查描述未来产物的代码片段。
            for target in re.findall(r"\[[^\]]*\]\(([^)]+)\)", content):
                if re.match(r"[a-zA-Z][a-zA-Z0-9+.-]*:", target) or target.startswith("#"):
                    continue
                clean = target.split("#", 1)[0]
                if clean and not (path.parent / clean).resolve().exists():
                    errors.append(f"本地Markdown链接失效：{path.relative_to(ROOT)} -> {target}")
        if args.checksums:
            manifest = safe_path("MANIFEST.sha256")
            if not manifest.is_file():
                errors.append("缺少 MANIFEST.sha256")
            else:
                for line in manifest.read_text(encoding="utf-8").splitlines():
                    digest, relative = line.split("  ", 1)
                    path = safe_path(relative)
                    if not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest() != digest:
                        errors.append(f"校验和不匹配：{relative}")
        if errors:
            for error in errors:
                print(f"ERROR: {error}", file=sys.stderr)
            return 1
        print(f"Urbino 提示词包结构与命名通过：{len(phases)}阶段，{len(rows)}条测试要求，{len(sources)}个参考来源。")
        print("此结果不表示网关代码已实现、已通过测试或可以上线。")
        return 0
    except (OSError, ValueError, KeyError, json.JSONDecodeError) as exc:
        print(f"校验失败：{exc}", file=sys.stderr)
        return 2

if __name__ == "__main__":
    raise SystemExit(main())
