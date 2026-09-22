#!/usr/bin/env python3
"""Detect drift in the upstream monobank API specifications.

The SDK is written against published specs. When one of them changes,
the code silently stops matching the API — that is how the payslip
delete endpoints ended up expecting HTTP 200 for an endpoint documented
as 204. This script pins each spec by a content hash so the drift shows
up as a failing job instead of a bug report.

Two kinds of source:

  * a plain file (corp-api) — hashed as served;
  * a ReDoc page, where the spec is embedded in the page as
    `__redoc_state`. The HTML wrapper changes for reasons that have
    nothing to do with the API, so the spec object is extracted and
    re-serialised canonically before hashing.

Usage:  check-spec-drift.py [--update]
"""
import hashlib
import json
import pathlib
import re
import sys
import urllib.request

UA = "go-monobank-sdk spec-drift check (+https://github.com/OlexiyOdarchuk/go-monobank-sdk)"
CHECKSUMS = pathlib.Path(__file__).resolve().parent.parent / "testdata" / "spec-checksums.json"

SOURCES = {
    "corp-api":   ("https://corp-api.monobank.ua/client-specification.yaml", "raw"),
    "personal":   ("https://api.monobank.ua/docs/index.html", "redoc"),
    "acquiring":  ("https://api.monobank.ua/docs/acquiring.html", "redoc"),
    "corporate":  ("https://api.monobank.ua/docs/corporate.html", "redoc"),
    "openbanking": ("https://ob.mono.bank/", "redoc"),
}


def fetch(url: str) -> bytes:
    req = urllib.request.Request(url, headers={"User-Agent": UA})
    with urllib.request.urlopen(req, timeout=60) as resp:
        return resp.read()


def redoc_spec(html: str) -> dict:
    """Pull the OpenAPI document out of a ReDoc page."""
    m = re.search(r"__redoc_state\s*=\s*", html)
    if not m:
        raise ValueError("no __redoc_state in page")
    start = html.index("{", m.end())
    depth, in_str, esc, i = 0, False, False, start
    while i < len(html):
        c = html[i]
        if in_str:
            if esc:
                esc = False
            elif c == "\\":
                esc = True
            elif c == '"':
                in_str = False
        elif c == '"':
            in_str = True
        elif c == "{":
            depth += 1
        elif c == "}":
            depth -= 1
            if depth == 0:
                break
        i += 1
    return json.loads(html[start:i + 1])["spec"]["data"]


def digest(name: str) -> tuple[str, str]:
    url, kind = SOURCES[name]
    body = fetch(url)
    if kind == "raw":
        return hashlib.sha256(body).hexdigest(), ""
    spec = redoc_spec(body.decode("utf-8", "replace"))
    canonical = json.dumps(spec, sort_keys=True, ensure_ascii=False).encode()
    return hashlib.sha256(canonical).hexdigest(), str(spec.get("info", {}).get("version", ""))


def main() -> int:
    update = "--update" in sys.argv
    known = json.loads(CHECKSUMS.read_text()) if CHECKSUMS.exists() else {}
    current, drifted = {}, []

    for name in SOURCES:
        try:
            sha, version = digest(name)
        except Exception as exc:  # noqa: BLE001 — any failure is worth reporting
            print(f"{name:12} ERROR  {exc}")
            drifted.append(name)
            continue
        current[name] = {"sha256": sha, "version": version}
        was = known.get(name, {})
        if update or not was:
            print(f"{name:12} recorded  {sha[:16]}  version={version or 'n/a'}")
        elif was.get("sha256") != sha:
            print(f"{name:12} DRIFTED   {was.get('sha256', '')[:16]} -> {sha[:16]}"
                  f"  version {was.get('version') or 'n/a'} -> {version or 'n/a'}")
            drifted.append(name)
        else:
            print(f"{name:12} unchanged {sha[:16]}  version={version or 'n/a'}")

    if update:
        CHECKSUMS.parent.mkdir(parents=True, exist_ok=True)
        CHECKSUMS.write_text(json.dumps(current, indent=2, ensure_ascii=False) + "\n")
        print(f"\nwrote {CHECKSUMS}")
        return 0

    if drifted:
        print("\nA published spec changed. Re-check the affected package against it,")
        print("then refresh the pins with: python3 scripts/check-spec-drift.py --update")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
