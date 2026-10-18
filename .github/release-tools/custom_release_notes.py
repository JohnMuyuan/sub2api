"""Generate personal release notes from upstream releases present in this build."""

import argparse
import json
from pathlib import Path
import re
import subprocess

UPSTREAM = "ranxi2001/sub2api"
UPSTREAM_REFS = "refs/custom-upstream/tags/"


def run(*args, check=True):
    result = subprocess.run(args, capture_output=True, text=True, encoding="utf-8")
    if check and result.returncode:
        raise RuntimeError(f"{args[0]} failed: {(result.stderr or result.stdout).strip()}")
    return result


def releases(repo):
    # Pagination keeps a long gap between personal releases from losing notes.
    pages = json.loads(run("gh", "api", f"repos/{repo}/releases?per_page=100", "--paginate", "--slurp").stdout)
    return sorted(
        (release for page in pages for release in page if not release["draft"] and not release["prerelease"]),
        key=lambda release: release["published_at"], reverse=True,
    )


def ancestor(ref, source):
    result = run("git", "merge-base", "--is-ancestor", f"{ref}^{{commit}}", source, check=False)
    if result.returncode not in (0, 1):
        raise RuntimeError(result.stderr.strip())
    return result.returncode == 0


def previous_release(own_releases, tag, source):
    for release in own_releases:
        ref = "refs/tags/" + release["tag_name"]
        if release["tag_name"] == tag:
            continue
        if run("git", "rev-parse", "--verify", "--quiet", ref, check=False).returncode == 0 and ancestor(ref, source):
            return ref
    return None


def changelog(body):
    # The upstream release template appends installation commands for its own
    # distribution. Keep change/upgrade notes and link the original for the rest.
    body = re.split(r"(?m)^## 📥 Installation\s*$", body or "", maxsplit=1)[0]
    return body.rstrip().removesuffix("---").rstrip()


def render(version, source, upstream_releases, previous):
    included = [release for release in upstream_releases if ancestor(UPSTREAM_REFS + release["tag_name"], source)]
    lines = ["江木源的自用版本", "", f"自用版本：v{version}  ·  构建提交：`{source}`", ""]
    if not included:
        lines += [f"当前构建未匹配到已合入的上游正式发布，参见[上游更新日志](https://github.com/{UPSTREAM}/releases)。"]
        return "\n".join(lines) + "\n"
    latest = included[0]
    lines += [f"上游基线：[{latest['tag_name']}]({latest['html_url']})", ""]
    changed = [release for release in included if previous and not ancestor(UPSTREAM_REFS + release["tag_name"], previous)]
    if changed:
        lines += ["## 本次合入的上游更新", ""]
        selected = list(reversed(changed))
    else:
        if previous:
            lines += ["本次没有新增上游正式版本，以下为当前基线版本的更新日志。", ""]
        selected = [latest]
    for release in selected:
        lines += [f"## 上游 {release['tag_name']}", "", f"[查看上游原始说明]({release['html_url']})", "", changelog(release["body"]) or "上游未填写该版本的更新说明。", ""]
    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--source", required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    upstream = releases(UPSTREAM)
    if upstream:
        # Keep upstream tags separate from the personal distribution's tags.
        refs = [f"refs/tags/{release['tag_name']}:{UPSTREAM_REFS}{release['tag_name']}" for release in upstream]
        run("git", "fetch", "--no-tags", "--force", f"https://github.com/{UPSTREAM}.git", *refs)
    previous = previous_release(releases(args.repo), "v" + args.version, args.source)
    args.output.write_text(render(args.version, args.source, upstream, previous), encoding="utf-8")


if __name__ == "__main__":
    main()
