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
from validate_omp import validate as validate_omp_files

ROOT = Path(__file__).resolve().parents[1]

def safe_path(relative: str) -> Path:
    path = (ROOT / relative).resolve()
    if not path.is_relative_to(ROOT):
        raise ValueError(f"越界路径：{relative}")
    return path

def load_json(relative: str):
    return json.loads(safe_path(relative).read_text(encoding="utf-8"))


def validate_project_identity(root: Path) -> list[str]:
    """检查命名契约；只检查本包规范，不能代替未来程序的功能测试。"""
    errors: list[str] = []
    try:
        project = json.loads((root / "project.json").read_text(encoding="utf-8"))
        state = json.loads((root / "progress/state.json").read_text(encoding="utf-8"))
        phases = json.loads((root / "phases.json").read_text(encoding="utf-8"))
        if not all(isinstance(item, dict) for item in (project, state, phases)):
            return ["项目标识、进度与阶段清单必须是 JSON 对象"]
        expected = {
            "project_name": "Urbino", "display_name": "Urbino",
            "chinese_name": None, "slug": "urbino",
            "repository_name": "urbino", "binary_name": "urbino",
            "windows_binary_name": "urbino.exe", "go_command_dir": "cmd/urbino",
            "default_config_file": "urbino.yaml",
            "example_config_file": "configs/urbino.example.yaml",
            "production_example_config_file": "configs/urbino.production.example.yaml",
            "environment_prefix": "URBINO_", "config_path_environment": "URBINO_CONFIG",
            "container_image_name": "urbino", "compose_project_name": "urbino",
            "compose_api_service": "urbino", "compose_worker_service": "urbino-worker",
            "user_agent_template": "urbino/<version>", "metrics_prefix": "urbino_",
            "otel_service_name": "urbino",
            "valkey_namespace_template": "urbino:<deployment-id>:",
        }
        for key, value in expected.items():
            if project.get(key) != value:
                errors.append(f"项目命名不一致：project.json {key} 应为 {value!r}")
        for key, value in {"project_name": "Urbino", "project_slug": "urbino",
                           "binary_name": "urbino", "schema_version": 2}.items():
            if state.get(key) != value:
                errors.append(f"进度项目标识不一致：{key} 应为 {value!r}")
        if "temporary_binary_name" in state:
            errors.append("进度仍含已废弃的临时二进制字段")
        if not project.get("pack_version") or project.get("pack_version") != phases.get("pack_version"):
            errors.append("project.json 与 phases.json 的提示词包版本不一致")
        module = project.get("go_module_default")
        if not isinstance(module, str) or not module or re.search(r"\s", module):
            errors.append("Go module 路径缺失或含空白")
        if project.get("go_module_is_placeholder") is True and module != "example.com/urbino":
            errors.append("未确认正式 module 时占位必须为 example.com/urbino")
        for phase in phases["phases"]:
            if "docs/14-project-identity.md" not in phase["required_docs"]:
                errors.append(f'阶段{phase["id"]}未引用统一命名规范')
            prompt_path = (root / phase["prompt"]).resolve()
            if not prompt_path.is_relative_to(root.resolve()):
                errors.append(f'阶段{phase["id"]}提示词路径越界')
                continue
            content = prompt_path.read_text(encoding="utf-8")
            for required in ("project.json", "docs/14-project-identity.md"):
                if required not in content:
                    errors.append(f'阶段{phase["id"]}提示词未声明必读{required}')
        # 只查本项目的命名用法；不把 AI Gateway 这类通用术语视作旧名称。
        forbidden = re.compile(
            r"Urbino[ _-]+Gateway|乌尔比诺|\b(?:bablo|relaro)\b|Hui[ _-]+AI[ _-]+Gateway|"
            r"cmd/gateway\b|example\.com/ai-gateway\b|\bGATEWAY_[A-Z_]+|"
            r"\bgateway\s+(?:serve|worker|migrate|bootstrap|admin|doctor|version)\b|"
            r"\bgateway(?:/|\.)ya?ml\b|\bgateway/<version>|"
            r"configs/config(?:\.production)?\.example\.yaml",
            re.IGNORECASE,
        )
        documents = [root / name for name in ("README.md", "AGENTS.md", "MASTER_PROMPT.md")]
        documents += sorted((root / "docs").glob("*.md"))
        documents += sorted((root / "prompts").glob("*.md"))
        documents += sorted((root / ".omp").rglob("*.md"))
        for document in documents:
            for number, line in enumerate(document.read_text(encoding="utf-8").splitlines(), 1):
                if forbidden.search(line):
                    errors.append(f"旧项目/临时命名残留：{document.relative_to(root)}:{number}")
    except (OSError, ValueError, KeyError, TypeError) as exc:
        errors.append(f"项目命名校验无法完成：{exc}")
    return errors

def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--checksums", action="store_true", help="同时校验发行包 MANIFEST.sha256；开发后文件改动会导致不匹配")
    args = parser.parse_args()
    errors: list[str] = []
    try:
        required = ["README.md", "AGENTS.md", "MASTER_PROMPT.md", "phases.json", "project.json",
                    "docs/14-project-identity.md", "CHANGELOG.md", "prompts/APPLY_NAMING_UPDATE.md",
                    "progress/state.json", "references/SOURCES.md", "references/sources.json",
                    "checklists/test-matrix.csv", "checklists/RELEASE_GATES.md",
                    "templates/stage-evidence.json", "prompts/REVIEW.md", "prompts/RESUME.md",
                    "prompts/REPAIR.md", "tools/compose_prompt.py", "tools/check_evidence.py",
                    "START_HERE.md", "omp/role-policy.json", "docs/15-omp-workflow.md",
                    "docs/16-omp-model-routing.md", "docs/17-omp-migration-recovery.md",
                    "tools/check_omp_runtime.py", "tools/migration_plan.py"]
        for relative in required:
            path = safe_path(relative)
            if not path.is_file() or path.stat().st_size == 0:
                errors.append(f"缺失或空文件：{relative}")
        errors.extend(validate_project_identity(ROOT))
        errors.extend(validate_omp_files(ROOT))
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
        states = load_json("progress/state.json")["stages"]
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
        for path in ROOT.rglob("*.md"):
            content = path.read_text(encoding="utf-8")
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
                listed: set[str] = set()
                for line in manifest.read_text(encoding="utf-8").splitlines():
                    digest, relative = line.split("  ", 1)
                    if relative in listed or not re.fullmatch(r"[0-9a-f]{64}", digest):
                        errors.append(f"重复或格式错误的摘要条目：{relative}")
                    listed.add(relative)
                    path = safe_path(relative)
                    if not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest() != digest:
                        errors.append(f"校验和不匹配：{relative}")
                actual = {p.relative_to(ROOT).as_posix() for p in ROOT.rglob("*")
                          if p.is_file() and p.name != "MANIFEST.sha256"
                          and not any(part in {".git", "__pycache__"} for part in p.relative_to(ROOT).parts)
                          and p.suffix != ".pyc"}
                if listed != actual:
                    errors.append("MANIFEST 与实际发行文件清单不同；开发后的工作目录不适用原始发行摘要")
        if errors:
            for error in errors:
                print(f"ERROR: {error}", file=sys.stderr)
            return 1
        print(f"Urbino 命名与提示词包结构通过：{len(phases)}阶段，{len(rows)}条测试要求，{len(sources)}个参考来源。")
        print("此结果不表示网关代码已实现、已通过测试或可以上线。")
        return 0
    except (OSError, ValueError, KeyError, json.JSONDecodeError) as exc:
        print(f"校验失败：{exc}", file=sys.stderr)
        return 2

if __name__ == "__main__":
    raise SystemExit(main())
