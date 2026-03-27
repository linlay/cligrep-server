#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import sys
from dataclasses import dataclass
from datetime import datetime, timezone
from email.utils import parsedate_to_datetime
from pathlib import Path
from typing import Iterable
from urllib.parse import quote
from urllib.request import Request, urlopen


USER_AGENT = "cligrep-upstream-import/1.0"
TIMEOUT_SECONDS = 120
FFMPEG_RELEASES_URL = "https://ffmpeg.org/releases/"


@dataclass
class Asset:
    name: str
    url: str


@dataclass
class Release:
    version: str
    published_at: datetime
    assets: list[Asset]


@dataclass
class Spec:
    slug: str
    kind: str
    owner: str = ""
    repo: str = ""
    package: str = ""


SPECS = [
    Spec(slug="gh", kind="github_release", owner="cli", repo="cli"),
    Spec(slug="playwright", kind="npm", package="playwright"),
    Spec(slug="vercel", kind="npm", package="vercel"),
    Spec(slug="supabase", kind="github_release", owner="supabase", repo="cli"),
    Spec(slug="ffmpeg", kind="ffmpeg_source"),
    Spec(slug="notebooklm", kind="pypi", package="notebooklm-py"),
]


def main() -> int:
    parser = argparse.ArgumentParser(description="Stage upstream CLI releases for CLI Grep.")
    parser.add_argument("--stage-root", required=True, help="Directory to populate with staged releases.")
    parser.add_argument("--manifest", required=True, help="Where to write the staging manifest JSON.")
    args = parser.parse_args()

    stage_root = Path(args.stage_root).resolve()
    manifest_path = Path(args.manifest).resolve()
    stage_root.mkdir(parents=True, exist_ok=True)

    results: list[dict[str, object]] = []
    failures: list[dict[str, str]] = []

    for spec in SPECS:
        slug_root = stage_root / spec.slug
        if slug_root.exists():
            shutil.rmtree(slug_root)
        try:
            releases = collect_releases(spec)
            if not releases:
                raise RuntimeError(f"expected at least 1 stable release for {spec.slug}, got {len(releases)}")
            stage_slug(stage_root, spec.slug, releases[:2])
            results.append(
                {
                    "slug": spec.slug,
                    "versions": [release.version for release in releases[:2]],
                    "assetCounts": {release.version: len(release.assets) for release in releases[:2]},
                }
            )
            print(f"staged {spec.slug}: {', '.join(release.version for release in releases[:2])}", file=sys.stderr)
        except Exception as exc:  # noqa: BLE001
            failures.append({"slug": spec.slug, "error": str(exc)})
            print(f"failed {spec.slug}: {exc}", file=sys.stderr)

    manifest = {
        "generatedAt": datetime.now(timezone.utc).isoformat(),
        "results": results,
        "failures": failures,
    }
    manifest_path.parent.mkdir(parents=True, exist_ok=True)
    manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")

    return 0 if results else 1


def collect_releases(spec: Spec) -> list[Release]:
    if spec.kind == "github_release":
        return collect_github_releases(spec)
    if spec.kind == "npm":
        return collect_npm_releases(spec)
    if spec.kind == "pypi":
        return collect_pypi_releases(spec)
    if spec.kind == "ffmpeg_source":
        return collect_ffmpeg_releases(spec)
    raise RuntimeError(f"unsupported spec kind {spec.kind}")


def collect_github_releases(spec: Spec) -> list[Release]:
    url = f"https://api.github.com/repos/{spec.owner}/{spec.repo}/releases?per_page=10"
    payload = fetch_json(url, headers=github_headers())

    releases: list[Release] = []
    for entry in payload:
        if entry.get("draft") or entry.get("prerelease"):
            continue
        assets = [
            Asset(name=asset["name"], url=asset["browser_download_url"])
            for asset in entry.get("assets", [])
            if asset.get("browser_download_url") and asset.get("name")
        ]
        if not assets:
            continue
        tag = str(entry.get("tag_name", "")).strip()
        if not tag:
            continue
        version = tag if tag.startswith("v") else f"v{tag}"
        published_at = parse_timestamp(entry.get("published_at") or entry.get("created_at"))
        releases.append(Release(version=version, published_at=published_at, assets=assets))
        if len(releases) == 2:
            break
    return releases


def collect_npm_releases(spec: Spec) -> list[Release]:
    url = f"https://registry.npmjs.org/{quote(spec.package, safe='')}"
    payload = fetch_json(url)
    time_map = payload.get("time", {})
    versions = []
    for version, metadata in payload.get("versions", {}).items():
        if not is_stable_numeric_version(version):
            continue
        tarball = metadata.get("dist", {}).get("tarball")
        if not tarball:
            continue
        versions.append(
            (
                version_key(version),
                Release(
                    version=f"v{version}",
                    published_at=parse_timestamp(time_map.get(version)),
                    assets=[Asset(name=os.path.basename(tarball), url=tarball)],
                ),
            )
        )
    versions.sort(key=lambda item: item[0], reverse=True)
    return [release for _, release in versions[:2]]


def collect_pypi_releases(spec: Spec) -> list[Release]:
    url = f"https://pypi.org/pypi/{quote(spec.package, safe='')}/json"
    payload = fetch_json(url)
    versions = []
    for version, files in payload.get("releases", {}).items():
        if not is_stable_numeric_version(version):
            continue
        assets = []
        published_at: datetime | None = None
        for file_entry in files:
            filename = file_entry.get("filename", "")
            file_url = file_entry.get("url", "")
            if not filename or not file_url:
                continue
            if not (filename.endswith(".whl") or filename.endswith(".tar.gz")):
                continue
            assets.append(Asset(name=filename, url=file_url))
            file_time = parse_timestamp(file_entry.get("upload_time_iso_8601") or file_entry.get("upload_time"))
            if published_at is None or file_time > published_at:
                published_at = file_time
        if not assets or published_at is None:
            continue
        versions.append((version_key(version), Release(version=f"v{version}", published_at=published_at, assets=assets)))
    versions.sort(key=lambda item: item[0], reverse=True)
    return [release for _, release in versions[:2]]


def collect_ffmpeg_releases(spec: Spec) -> list[Release]:
    del spec
    html = fetch_text(FFMPEG_RELEASES_URL)
    matches = re.findall(r'href="(ffmpeg-([0-9]+(?:\.[0-9]+)*)\.tar\.xz)"', html)
    by_version: dict[str, str] = {}
    for filename, version in matches:
        by_version[version] = f"{FFMPEG_RELEASES_URL}{filename}"

    releases = []
    for version in sorted(by_version.keys(), key=version_key, reverse=True)[:2]:
        url = by_version[version]
        releases.append(
            Release(
                version=f"v{version}",
                published_at=head_last_modified(url),
                assets=[Asset(name=os.path.basename(url), url=url)],
            )
        )
    return releases


def stage_slug(stage_root: Path, slug: str, releases: Iterable[Release]) -> None:
    slug_root = stage_root / slug
    slug_root.mkdir(parents=True, exist_ok=True)

    staged_releases = list(releases)
    for release in staged_releases:
        version_dir = slug_root / release.version
        version_dir.mkdir(parents=True, exist_ok=True)
        for asset in release.assets:
            target = version_dir / asset.name
            download_file(asset.url, target)
            touch_path(target, release.published_at)
        checksum_file = version_dir / f"{slug}_{release.version}_checksums.txt"
        write_checksum_file(version_dir, checksum_file)
        touch_path(checksum_file, release.published_at)

    latest_dir = slug_root / "latest"
    if latest_dir.exists() and not latest_dir.is_symlink():
        shutil.rmtree(latest_dir)
    elif latest_dir.is_symlink():
        latest_dir.unlink()
    latest_dir.mkdir(parents=True, exist_ok=True)

    newest = staged_releases[0]
    newest_dir = slug_root / newest.version
    for entry in sorted(newest_dir.iterdir()):
        link_name = "checksums.txt" if is_checksum_name(entry.name) else entry.name
        link_path = latest_dir / link_name
        if link_path.exists() or link_path.is_symlink():
            link_path.unlink()
        link_path.symlink_to(Path("..") / newest.version / entry.name)


def write_checksum_file(version_dir: Path, checksum_path: Path) -> None:
    lines = []
    for entry in sorted(version_dir.iterdir()):
        if entry == checksum_path or not entry.is_file():
            continue
        lines.append(f"{sha256_file(entry)}  {entry.name}")
    checksum_path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def download_file(url: str, target: Path) -> None:
    request = Request(url, headers={"User-Agent": USER_AGENT})
    with urlopen(request, timeout=TIMEOUT_SECONDS) as response, target.open("wb") as fh:
        shutil.copyfileobj(response, fh)


def fetch_json(url: str, headers: dict[str, str] | None = None) -> object:
    request = Request(url, headers=default_headers(headers))
    with urlopen(request, timeout=TIMEOUT_SECONDS) as response:
        return json.load(response)


def fetch_text(url: str, headers: dict[str, str] | None = None) -> str:
    request = Request(url, headers=default_headers(headers))
    with urlopen(request, timeout=TIMEOUT_SECONDS) as response:
        return response.read().decode("utf-8")


def head_last_modified(url: str) -> datetime:
    request = Request(url, headers=default_headers(None), method="HEAD")
    with urlopen(request, timeout=TIMEOUT_SECONDS) as response:
        header = response.headers.get("Last-Modified")
    if not header:
        return datetime.now(timezone.utc)
    return parsedate_to_datetime(header).astimezone(timezone.utc)


def default_headers(extra: dict[str, str] | None) -> dict[str, str]:
    headers = {"User-Agent": USER_AGENT}
    if extra:
        headers.update(extra)
    return headers


def github_headers() -> dict[str, str]:
    headers = {"Accept": "application/vnd.github+json"}
    token = os.environ.get("GITHUB_TOKEN", "").strip()
    if token:
        headers["Authorization"] = f"Bearer {token}"
    return headers


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def parse_timestamp(value: object) -> datetime:
    if not value:
        return datetime.now(timezone.utc)
    text = str(value).strip()
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    return datetime.fromisoformat(text).astimezone(timezone.utc)


def touch_path(path: Path, timestamp: datetime) -> None:
    epoch = timestamp.timestamp()
    os.utime(path, (epoch, epoch))


def is_stable_numeric_version(value: str) -> bool:
    return re.fullmatch(r"[0-9]+(?:\.[0-9]+)+", value) is not None


def version_key(value: str) -> tuple[int, ...]:
    return tuple(int(part) for part in value.split("."))


def is_checksum_name(name: str) -> bool:
    lowered = name.lower()
    return "checksum" in lowered and lowered.endswith(".txt")


if __name__ == "__main__":
    raise SystemExit(main())
