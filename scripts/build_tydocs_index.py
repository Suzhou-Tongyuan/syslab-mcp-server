import argparse
import html
import json
import os
import re
import unicodedata
from pathlib import Path, PurePosixPath


PERSISTED_INDEX_VERSION = "v1"
PERSISTED_INDEX_FILENAME = "tydocs-index.json"
FUNCTION_MAPPING_FILENAME = "函数映射表.json"
DOC_EXTENSIONS = {".html", ".htm", ".md", ".txt"}
SYMBOL_PATTERN = re.compile(r"^[A-Za-z_][A-Za-z0-9_!?.]*$")
TAG_PATTERN = re.compile(r"(?s)<[^>]*>")
COMMENT_PATTERN = re.compile(r"(?s)<!--.*?-->")
WHITESPACE_PATTERN = re.compile(r"\s+")
PLACEHOLDER_LINES = {"---"}


def compact_text(text: str) -> str:
    lines = text.splitlines()
    cleaned = []
    for line in lines:
        line = WHITESPACE_PATTERN.sub(" ", line.strip())
        if line:
            cleaned.append(line)
    return "\n".join(cleaned)


def strip_html(text: str) -> str:
    text = TAG_PATTERN.sub("\n", text)
    return html.unescape(text)


def normalize_visible_text(text: str) -> str:
    text = html.unescape(text.replace("&nbsp;", " "))
    text = TAG_PATTERN.sub(" ", text)
    return WHITESPACE_PATTERN.sub(" ", text).strip()


def iter_visible_lines(text: str):
    text = COMMENT_PATTERN.sub("\n", text)
    for raw_line in text.splitlines():
        line = normalize_visible_text(raw_line)
        if not line or line in PLACEHOLDER_LINES:
            continue
        yield line


def read_doc_text(path: Path) -> str:
    text = path.read_text(encoding="utf-8-sig", errors="replace")
    if path.suffix.lower() in {".html", ".htm"}:
        text = strip_html(text)
    return compact_text(text)


def extract_title(text: str, path: Path) -> str:
    for line in iter_visible_lines(text):
        line = line.lstrip("#").strip()
        if len(line) >= 2:
            return line
    return path.stem


def extract_summary(text: str, title: str) -> str:
    for line in iter_visible_lines(text):
        if line.lstrip("# ").strip().lower() == title.lower():
            continue
        return line[:240]
    return ""


def infer_symbol(path: Path, title: str) -> str:
    stem = unicodedata.normalize("NFKC", path.stem)
    if SYMBOL_PATTERN.match(stem):
        return stem
    parts = title.split()
    if parts:
        first = unicodedata.normalize("NFKC", parts[0])
        if SYMBOL_PATTERN.match(first):
            return first
    return ""


def unique_strings(values):
    seen = set()
    out = []
    for value in values:
        value = str(value).strip()
        if not value:
            continue
        key = value.lower()
        if key in seen:
            continue
        seen.add(key)
        out.append(value)
    return out


def first_existing_file(paths):
    for candidate in paths:
        if candidate.is_file():
            return str(candidate.resolve())
    return ""


def choose_projects_root(ai_assets_root: Path) -> Path:
    for candidate in (
        ai_assets_root / "projects",
        ai_assets_root / "syslabHelpSourceCode" / "projects",
    ):
        if candidate.is_dir():
            return candidate
    return ai_assets_root / "projects"


def choose_function_table_path(ai_assets_root: Path) -> Path:
    for candidate in (
        ai_assets_root / "static" / "FunctionTable" / FUNCTION_MAPPING_FILENAME,
        ai_assets_root / "SearchCenter" / "static" / "FunctionTable" / FUNCTION_MAPPING_FILENAME,
    ):
        if candidate.is_file():
            return candidate
    return ai_assets_root / "static" / "FunctionTable" / FUNCTION_MAPPING_FILENAME


def is_placeholder_summary(summary: str) -> bool:
    return not summary or summary.strip() in PLACEHOLDER_LINES


def discover_packages(projects_root: Path):
    if not projects_root.is_dir():
        return []
    packages = []
    for item in sorted(projects_root.iterdir()):
        if not item.is_dir():
            continue
        docs_path = ""
        for candidate in (item / "Doc", item / "doc", item):
            if candidate.is_dir():
                docs_path = str(candidate.resolve())
                break
        packages.append(
            {
                "name": item.name,
                "docs_path": docs_path,
                "docs_source": "syslab_aiassets",
                "has_docs": bool(docs_path),
            }
        )
    return packages


def index_package_docs(package):
    docs_path = Path(package["docs_path"])
    entries = []
    for path in sorted(docs_path.rglob("*")):
        if not path.is_file() or path.suffix.lower() not in DOC_EXTENSIONS:
            continue
        try:
            text = read_doc_text(path)
        except OSError:
            continue
        title = extract_title(text, path)
        summary = extract_summary(text, title)
        entries.append(
            {
                "package": package["name"],
                "title": title,
                "symbol": infer_symbol(path, title),
                "summary": summary,
                "path": str(path.resolve()),
                "format": path.suffix.lower().lstrip("."),
                "source": package["docs_source"],
            }
        )
    return entries


def load_function_mappings(function_table_path: Path):
    if not function_table_path.is_file():
        return []
    return json.loads(function_table_path.read_text(encoding="utf-8-sig", errors="replace"))


def resolve_mapping_doc_path(projects_root: Path, mapping):
    package_name = str(mapping.get("package", "")).strip()
    help_url = str(mapping.get("helpUrl", "")).strip()
    if not package_name or not help_url:
        return ""

    relative = PurePosixPath(help_url.lstrip("/"))
    primary = projects_root / package_name / Path(*relative.parts)
    base = primary.with_suffix("")
    return first_existing_file(
        [
            primary,
            base.with_suffix(".md"),
            base.with_suffix(".html"),
            base.with_suffix(".htm"),
            base.with_suffix(".txt"),
        ]
    )


def apply_function_mappings(entries, function_table_path: Path, projects_root: Path):
    by_path = {entry["path"].lower(): index for index, entry in enumerate(entries)}
    for mapping in load_function_mappings(function_table_path):
        resolved = resolve_mapping_doc_path(projects_root, mapping)
        if not resolved:
            continue
        aliases = unique_strings([mapping.get("name", ""), mapping.get("matlabFunction", "")])
        if resolved.lower() in by_path:
            entry = entries[by_path[resolved.lower()]]
            if is_placeholder_summary(entry.get("summary", "")) and str(mapping.get("description", "")).strip():
                entry["summary"] = str(mapping.get("description", "")).strip()
            entry["aliases"] = unique_strings(entry.get("aliases", []) + aliases)
            continue

        path = Path(resolved)
        try:
            text = read_doc_text(path)
        except OSError:
            continue
        title = extract_title(text, path)
        summary = extract_summary(text, title) or str(mapping.get("description", "")).strip()
        entry = {
            "package": str(mapping.get("package", "")).strip(),
            "title": title,
            "symbol": str(mapping.get("name", "")).strip() or infer_symbol(path, title),
            "summary": summary,
            "path": str(path.resolve()),
            "format": path.suffix.lower().lstrip("."),
            "source": "function_mapping",
            "aliases": aliases,
        }
        by_path[entry["path"].lower()] = len(entries)
        entries.append(entry)
    return entries


def build_index(ai_assets_root: Path):
    projects_root = choose_projects_root(ai_assets_root)
    function_table_path = choose_function_table_path(ai_assets_root)

    packages = discover_packages(projects_root)
    entries = []
    for package in packages:
        if package["has_docs"]:
            entries.extend(index_package_docs(package))
    entries = apply_function_mappings(entries, function_table_path, projects_root)

    packages.sort(key=lambda item: item["name"])
    entries.sort(key=lambda item: (item["package"], item["path"]))
    return {"packages": packages, "entries": entries}


def relativize_index_paths(index, output_dir: Path):
    def convert(path_value: str) -> str:
        return Path(os.path.relpath(str(Path(path_value).resolve()), str(output_dir))).as_posix()

    packages = []
    for package in index["packages"]:
        item = dict(package)
        if item.get("docs_path"):
            item["docs_path"] = convert(item["docs_path"])
        packages.append(item)

    entries = []
    for entry in index["entries"]:
        item = dict(entry)
        item["path"] = convert(item["path"])
        entries.append(item)

    return {"packages": packages, "entries": entries}


def main() -> int:
    parser = argparse.ArgumentParser(description="Build tydocs-index.json from an AIAssets folder")
    parser.add_argument("--ai-assets-root", required=True, help="Path to the AIAssets folder")
    parser.add_argument(
        "--output",
        default="",
        help="Output file path; defaults to <ai-assets-root>/tydocs-index.json",
    )
    args = parser.parse_args()

    ai_assets_root = Path(args.ai_assets_root).resolve()
    output_path = Path(args.output).resolve() if args.output else ai_assets_root / PERSISTED_INDEX_FILENAME

    index = build_index(ai_assets_root)
    payload = {"version": PERSISTED_INDEX_VERSION, **relativize_index_paths(index, output_path.parent)}
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps(payload, ensure_ascii=False), encoding="utf-8")

    print(f"generated {output_path} (packages={len(index['packages'])}, entries={len(index['entries'])})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
