"""Generate release notes from commit subjects since the previous published release."""

import json
import os
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import quote


def git(*args):
    return subprocess.check_output(["git", *args], text=True, encoding="utf-8").strip()


def generate(repository, tag, sha, releases):
    previous = None
    for release in sorted(releases, key=lambda r: r.get("published_at") or "", reverse=True):
        candidate = release["tag_name"]
        if release["draft"] or candidate == tag or ("-" not in tag and release["prerelease"]):
            continue
        ref = f"refs/tags/{candidate}^{{commit}}"
        resolved = subprocess.run(["git", "rev-parse", "--verify", ref], capture_output=True, text=True)
        if resolved.returncode or resolved.stdout.strip() == sha:
            continue
        if subprocess.run(["git", "merge-base", "--is-ancestor", ref, sha]).returncode == 0:
            previous = candidate
            break
    revision = f"refs/tags/{previous}..{sha}" if previous else sha
    entries = git("log", "--no-merges", "--reverse", "--format=%H%x09%s", revision)
    lines = ["## 更新内容", ""]
    for entry in entries.splitlines():
        commit, subject = entry.split("\t", 1)
        subject = re.sub(r"([\\`*_{}\[\]()<>#!|~])", r"\\\1", subject)
        lines.append(f"- {subject} ([{commit[:7]}](https://github.com/{repository}/commit/{commit}))")
    if not entries:
        lines.append("- 无新增提交。")
    if previous:
        comparison = f"https://github.com/{repository}/compare/{quote(previous, safe='')}...{quote(tag, safe='')}"
        lines.extend(["", f"**完整变更**：[{previous}…{tag}]({comparison})"])
    return "\n".join(lines) + "\n"


if __name__ == "__main__":
    repository = os.environ["GITHUB_REPOSITORY"]
    pages = json.loads(subprocess.check_output(
        ["gh", "api", "--paginate", "--slurp", f"repos/{repository}/releases?per_page=100"],
        text=True, encoding="utf-8",
    ))
    releases = [release for page in pages for release in page]
    Path(sys.argv[1]).write_text(generate(
        repository, os.environ["RELEASE_TAG"], os.environ["RELEASE_SHA"], releases,
    ), encoding="utf-8")
