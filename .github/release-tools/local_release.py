#!/usr/bin/env python3
"""Cross-compile frontend + Linux amd64 backend locally and publish a GitHub Release.

Avoids GitHub Actions completely while producing the exact archive layout,
embedded metadata, checksums, and release notes expected by Sub2API's built-in
updater and managed login runtime loader.
"""

from __future__ import annotations

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[2]
DEFAULT_REPO = "JohnMuyuan/sub2api"


def run(cmd: list[str], *, cwd: Path | None = None, env: dict[str, str] | None = None, capture: bool = False) -> str:
    print(f"+ {' '.join(cmd)}", flush=True)
    res = subprocess.run(
        cmd,
        cwd=str(cwd) if cwd else None,
        env=env,
        check=True,
        text=True,
        encoding="utf-8",
        capture_output=capture,
    )
    return res.stdout.strip() if capture else ""


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def next_version(repo: str) -> str:
    now = datetime.now(timezone.utc)
    prefix = f"{now.year}.{now.month}."
    raw = run(["gh", "release", "list", "--repo", repo, "--limit", "20", "--json", "tagName"], capture=True)
    tags = [item.get("tagName", "") for item in json.loads(raw or "[]")]
    max_seq = 0
    for tag in tags:
        v = tag.lstrip("v")
        parts = v.split(".")
        if len(parts) == 3 and parts[2].isdigit():
            max_seq = max(max_seq, int(parts[2]))
    return f"{prefix}{max_seq + 1}"


def package_linux_archive(binary_path: Path, resources_dir: Path, output_tar_gz: Path) -> None:
    def add_path(tf: tarfile.TarFile, src: Path, arcname: str, mode: int) -> None:
        info = tf.gettarinfo(str(src), arcname=arcname)
        info.uid = 0
        info.gid = 0
        info.uname = "root"
        info.gname = "root"
        info.mode = mode
        if src.is_file():
            with src.open("rb") as fh:
                tf.addfile(info, fh)
        else:
            tf.addfile(info)

    with tarfile.open(output_tar_gz, "w:gz", format=tarfile.GNU_FORMAT) as tf:
        add_path(tf, binary_path, "sub2api", 0o755)
        if resources_dir.exists():
            add_path(tf, resources_dir, "resources", 0o755)
            for item in sorted(resources_dir.rglob("*")):
                rel = item.relative_to(resources_dir).as_posix()
                arcname = f"resources/{rel}"
                if item.is_dir():
                    add_path(tf, item, arcname, 0o755)
                elif item.is_file():
                    add_path(tf, item, arcname, 0o644)


def fetch_latest_reauth_archive(repo: str, dest_dir: Path, target_version: str) -> Path:
    latest_raw = run(["gh", "release", "view", "--repo", repo, "--json", "tagName,assets"], capture=True)
    latest = json.loads(latest_raw)
    tag = latest["tagName"]
    assets = latest.get("assets", [])
    reauth_asset = next((a for a in assets if a["name"].startswith("sub2api-reauth_") and a["name"].endswith("_linux_amd64.tar.gz")), None)
    if not reauth_asset:
        raise RuntimeError(f"No linux_amd64 reauth archive found on latest release {tag}")

    dl_dir = dest_dir / "prev-release"
    dl_dir.mkdir(parents=True, exist_ok=True)
    run(["gh", "release", "download", tag, "--repo", repo, "-p", reauth_asset["name"], "-p", "checksums.txt", "-D", str(dl_dir)])

    downloaded = dl_dir / reauth_asset["name"]
    checksums_path = dl_dir / "checksums.txt"
    expected = None
    for line in checksums_path.read_text(encoding="utf-8").splitlines():
        parts = line.strip().split()
        if len(parts) == 2 and parts[1] == reauth_asset["name"]:
            expected = parts[0].lower()
            break
    actual = sha256_file(downloaded).lower()
    if expected and actual != expected:
        raise RuntimeError(f"SHA256 mismatch on downloaded {reauth_asset['name']}: expected {expected}, got {actual}")

    target_path = dest_dir / f"sub2api-reauth_{target_version}_linux_amd64.tar.gz"
    shutil.copy2(downloaded, target_path)
    return target_path


def main() -> int:
    parser = argparse.ArgumentParser(description="Build and publish a Sub2API custom release locally.")
    parser.add_argument("--repo", default=DEFAULT_REPO, help="Target GitHub repo (default: JohnMuyuan/sub2api)")
    parser.add_argument("--version", default="", help="Release version without leading v (e.g. 2026.11.50). Auto-increments if omitted.")
    parser.add_argument("--reauth-archive", default="", help="Optional local path to sub2api-reauth_*_linux_amd64.tar.gz")
    parser.add_argument("--publish", action="store_true", help="Upload and publish the GitHub Release via gh")
    args = parser.parse_args()

    source_sha = run(["git", "rev-parse", "HEAD"], cwd=ROOT, capture=True)
    version = args.version.strip().lstrip("v") or next_version(args.repo)
    build_date = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    tag = f"v{version}"

    print(f"Building Sub2API {version} from commit {source_sha} for {args.repo}...", flush=True)

    # 1. Build frontend into backend/internal/web/dist
    pnpm_cmd = "pnpm.cmd" if os.name == "nt" else "pnpm"
    fe_env = os.environ.copy()
    fe_env["VITE_UPDATE_REPO"] = args.repo
    run([pnpm_cmd, "run", "build"], cwd=ROOT / "frontend", env=fe_env)

    with tempfile.TemporaryDirectory(prefix="sub2api-local-release-") as tmp:
        stage = Path(tmp)
        binary_path = stage / "sub2api"

        # 2. Cross-compile pure-Go Linux amd64 binary with embedded frontend
        go_env = os.environ.copy()
        go_env["CGO_ENABLED"] = "0"
        go_env["GOOS"] = "linux"
        go_env["GOARCH"] = "amd64"
        ldflags = " ".join([
            "-s -w",
            f"-X main.Version={version}",
            f"-X main.Commit={source_sha}",
            f"-X main.Date={build_date}",
            "-X main.BuildType=release",
            f"-X github.com/Wei-Shaw/sub2api/internal/service.githubRepo={args.repo}",
            f"-X github.com/Wei-Shaw/sub2api/internal/reauthruntime.githubRepo={args.repo}",
        ])
        run(
            ["go", "build", "-tags", "embed", f"-ldflags={ldflags}", "-trimpath", "-o", str(binary_path), "./cmd/server"],
            cwd=ROOT / "backend",
            env=go_env,
        )

        # Verify ELF header
        magic = binary_path.read_bytes()[:4]
        if magic != b"\x7fELF":
            raise RuntimeError(f"Built binary is not a Linux ELF executable (magic={magic!r})")

        # 3. Package sub2api_<version>_linux_amd64.tar.gz
        app_archive = stage / f"sub2api_{version}_linux_amd64.tar.gz"
        package_linux_archive(binary_path, ROOT / "backend" / "resources", app_archive)

        # Verify archive contents
        with tarfile.open(app_archive, "r:gz") as tf:
            names = tf.getnames()
            if "sub2api" not in names:
                raise RuntimeError("Packaged archive is missing top-level sub2api binary")
            info = tf.getmember("sub2api")
            if (info.mode & 0o111) == 0:
                raise RuntimeError(f"sub2api binary inside archive is not executable (mode={oct(info.mode)})")

        # 4. Obtain matching reauth runtime archive
        if args.reauth_archive:
            reauth_archive = stage / f"sub2api-reauth_{version}_linux_amd64.tar.gz"
            shutil.copy2(Path(args.reauth_archive), reauth_archive)
        else:
            reauth_archive = fetch_latest_reauth_archive(args.repo, stage, version)

        # 5. Compute checksums.txt
        checksums_path = stage / "checksums.txt"
        checksums_lines = [
            f"{sha256_file(app_archive)}  {app_archive.name}",
            f"{sha256_file(reauth_archive)}  {reauth_archive.name}",
        ]
        checksums_path.write_text("\n".join(checksums_lines) + "\n", encoding="utf-8")

        # 6. Generate release notes using the repo's release-notes tool
        notes_path = stage / "custom-release-notes.md"
        run(
            [
                sys.executable,
                str(ROOT / ".github" / "release-tools" / "custom_release_notes.py"),
                "--repo", args.repo,
                "--version", version,
                "--source", source_sha,
                "--output", str(notes_path),
            ],
            cwd=ROOT,
        )

        print(f"Prepared assets:\n  {app_archive.name} ({app_archive.stat().st_size:,} bytes)\n  {reauth_archive.name} ({reauth_archive.stat().st_size:,} bytes)", flush=True)
        print("Checksums:\n" + checksums_path.read_text(encoding="utf-8"), flush=True)

        if not args.publish:
            print("Dry run complete (pass --publish to create the GitHub Release).", flush=True)
            return 0

        # 7. Create draft release, upload assets, then publish as latest
        exists = subprocess.run(
            ["gh", "release", "view", tag, "--repo", args.repo],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        ).returncode == 0
        if not exists:
            run([
                "gh", "release", "create", tag,
                "--repo", args.repo,
                "--target", source_sha,
                "--draft",
                "--title", f"Custom {version}",
                "--notes-file", str(notes_path),
            ])
        run(["gh", "release", "edit", tag, "--notes-file", str(notes_path), "--repo", args.repo])
        run([
            "gh", "release", "upload", tag,
            str(app_archive), str(reauth_archive), str(checksums_path),
            "--clobber", "--repo", args.repo,
        ])
        run(["gh", "release", "edit", tag, "--draft=false", "--latest", "--repo", args.repo])
        print(f"Published release {tag} ({args.repo}) from {source_sha}.", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
