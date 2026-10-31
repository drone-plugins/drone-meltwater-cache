#!/usr/bin/env python3
"""Helpers for cache-intel.sh: research fields, document gates, fixture commits and reports.

Subcommands:
  fields <research.md>                    print `name<TAB>value` for every field
  check <research|design> <file>          print problems; exit 1 when the document is incomplete
  fixture-commit --from DIR --out FILE    build a Harness Code commit body (MCP harness_create commit)
  report --work DIR --repo DIR ...        render REPORT.md, PR_BODY.md and HARNESS_CORE_PR.md
"""
import argparse
import base64
import json
import os
import re
import sys

FIELD_RE = re.compile(r"^- ([a-z_]+):[ \t]*(.*?)\s*$")
URL_RE = re.compile(r"https?://\S+")
ENV_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*=[^/\s][^\s]*$")

REQUIRED_FIELDS = [
    "tool_id", "tool_versions", "detect_files", "key_inputs", "cache_paths", "excluded_paths",
    "strategy", "cache_env", "cache_env_reason", "cache_env_side_effects", "fixture_repo", "fixture_ref", "install_image",
    "install_cmd", "warm_check", "invalidate_file",
]
OPTIONAL_FIELDS = {"fixture_subdir": ".", "offline_cmd": "none", "unset_env": "none", "harness_code_repo": "none"}

GATES = [
    ("research", "Research: cache layout, key, CI integration, sources"),
    ("design", "Design: mapping entry, paths, scenario matrix"),
    ("create", "Implementation and focused unit tests"),
    ("test-local", "Package tests, go vet, go build"),
    ("test-compatibility", "Scenario and legacy-key compatibility tests"),
    ("test-lifecycle", "In-repo fixture lifecycle"),
    ("test-public", "Public project: cold, warm, offline, invalidation (local)"),
    ("build", "Cross-compiled release binaries"),
    ("finalize", "README and CHANGELOG"),
    ("harness-core", "harness-core env injection"),
    ("remote-cold", "Harness: cold run"),
    ("remote-warm", "Harness: warm run"),
    ("remote-invalidate", "Harness: invalidation run"),
    ("product-seed", "Harness Cache Intelligence (product path): seed run"),
    ("product-warm", "Harness Cache Intelligence (product path): warm run"),
]


# ---------------------------------------------------------------- documents

def read(path):
    try:
        with open(path, encoding="utf-8") as f:
            return f.read()
    except FileNotFoundError:
        return ""


def sections(text):
    """Map `## heading` to its body (without the heading line)."""
    out, name, buf = {}, None, []
    for line in text.splitlines():
        if line.startswith("## "):
            if name is not None:
                out[name] = "\n".join(buf).strip()
            name, buf = line[3:].strip(), []
        elif name is not None:
            buf.append(line)
    if name is not None:
        out[name] = "\n".join(buf).strip()
    return out


def strip_comments(text):
    return re.sub(r"<!--.*?-->", "", text, flags=re.S)


def parse_fields(text):
    fields = dict(OPTIONAL_FIELDS)
    body = sections(strip_comments(text)).get("Fields", "")
    for line in body.splitlines():
        m = FIELD_RE.match(line)
        if m:
            fields[m.group(1)] = m.group(2).strip().strip("`")
    return fields


def check_research(text):
    problems = []
    clean = strip_comments(text)
    fields = parse_fields(text)
    for name in REQUIRED_FIELDS:
        v = fields.get(name, "")
        if not v or "TODO" in v:
            problems.append("field %s is not filled" % name)
    env = fields.get("cache_env", "")
    if env and "TODO" not in env and env != "none" and not ENV_RE.match(env):
        problems.append("cache_env must be none or KEY=workspace-relative-dir (got %r)" % env)
    if env not in ("", "none") and fields.get("cache_env_reason", "none") in ("", "none"):
        problems.append("cache_env_reason is required when cache_env is set")
    if env not in ("", "none") and fields.get("cache_env_side_effects", "none") in ("", "none"):
        problems.append("cache_env_side_effects is required when cache_env is set: list what else the variable moves (config, credentials, installed binaries)")
    if fields.get("strategy") not in ("follow-only", "relocate") and "TODO" not in fields.get("strategy", "TODO"):
        problems.append("strategy must be follow-only or relocate")
    repo = fields.get("fixture_repo", "")
    if repo and "TODO" not in repo and not repo.startswith(("https://", "http://")):
        problems.append("fixture_repo must be a public https git URL (use harness_code_repo for Harness Code fixtures)")
    for name in ("warm_check", "invalidate_file", "fixture_subdir"):
        v = fields.get(name, "")
        if v.startswith("/") or v.startswith(".."):
            problems.append("%s must be workspace-relative" % name)
    parts = sections(clean)
    for heading in parts:
        if "TODO" in parts[heading] and heading != "Fields":
            problems.append("section '%s' still contains TODO" % heading)
    if len(URL_RE.findall(parts.get("Sources", ""))) < 2:
        problems.append("section 'Sources' needs at least two URLs")
    return problems


def check_design(text):
    problems = []
    secs = sections(strip_comments(text))
    for heading in ("Summary", "Mapping entry", "Paths", "Scenario matrix", "Compatibility and migration", "Limitations"):
        if heading not in secs:
            problems.append("missing section '%s'" % heading)
        elif not secs[heading].strip():
            problems.append("section '%s' is empty" % heading)
    for heading, body in secs.items():
        if "TODO" in body:
            problems.append("section '%s' still contains TODO" % heading)
    return problems


# ---------------------------------------------------------------- fixture commit

def fixture_commit(src, out, title, branch, files, max_kb):
    selected = []
    roots = [os.path.join(src, f) for f in files] if files else [src]
    for root in roots:
        if os.path.isfile(root):
            selected.append(root)
            continue
        for dirpath, dirnames, filenames in os.walk(root):
            dirnames[:] = [d for d in dirnames if d not in (".git", "target", "node_modules", ".cache")]
            selected.extend(os.path.join(dirpath, n) for n in filenames)
    actions, total, skipped = [], 0, []
    for path in sorted(set(selected)):
        rel = os.path.relpath(path, src).replace(os.sep, "/")
        size = os.path.getsize(path)
        if size > max_kb * 1024:
            skipped.append(rel)
            continue
        with open(path, "rb") as f:
            data = f.read()
        try:
            text = data.decode("utf-8")
            if "\x00" in text:
                raise UnicodeDecodeError("utf-8", b"", 0, 1, "nul")
            actions.append({"action": "CREATE", "path": rel, "payload": text, "encoding": "utf8"})
        except UnicodeDecodeError:
            actions.append({"action": "CREATE", "path": rel, "payload": base64.b64encode(data).decode(), "encoding": "base64"})
        total += size
    with open(out, "w", encoding="utf-8") as f:
        json.dump({"title": title, "branch": branch, "actions": actions}, f)
    print("FIXTURE_COMMIT=%s files=%d bytes=%d skipped=%s" % (out, len(actions), total, ",".join(skipped) or "none"))


# ---------------------------------------------------------------- report

def read_kv(path):
    out = {}
    for line in read(path).splitlines():
        if "=" in line and not line.startswith("#"):
            k, v = line.split("=", 1)
            out[k.strip()] = v.strip()
    return out


def read_ledger(path):
    rows = []
    for line in read(path).splitlines():
        parts = line.split("\t")
        if len(parts) >= 4:
            rows.append({"at": parts[0], "gate": parts[1], "result": parts[2], "evidence": parts[3]})
    return rows


def read_runs(path):
    runs = []
    for line in read(path).splitlines():
        parts = line.split("\t")
        if len(parts) >= 5:
            metrics = dict(kv.split("=", 1) for kv in (parts[5].split(";") if len(parts) > 5 and parts[5] else []) if "=" in kv)
            runs.append({"at": parts[0], "run": parts[1], "result": parts[2], "url": parts[3], "image": parts[4], "metrics": metrics})
    return runs


def demote(md):
    return re.sub(r"^(#+) ", lambda m: "#" + m.group(1) + " ", md, flags=re.M)


def code(v):
    return "`%s`" % v if v and v != "none" else "none"


def link(label, url):
    return "[%s](%s)" % (label, url) if url else label


def kb_to_mb(kb):
    try:
        return "%.1f MB" % (float(kb) / 1024.0)
    except (TypeError, ValueError):
        return "n/a"


def secs(v):
    try:
        return "%.1fs" % float(v)
    except (TypeError, ValueError):
        return "n/a"


class Report:
    def __init__(self, a):
        self.a = a
        w = a.work
        self.fields = parse_fields(read(os.path.join(w, "research.md")))
        self.research = sections(strip_comments(read(os.path.join(w, "research.md"))))
        self.design = sections(strip_comments(read(os.path.join(w, "design.md"))))
        self.state = read_kv(os.path.join(w, "state.env"))
        self.metrics = read_kv(os.path.join(w, "metrics.env"))
        self.ledger = read_ledger(os.path.join(w, "ledger.tsv"))
        for r in self.ledger:
            r["evidence"] = r["evidence"].replace(w.rstrip("/"), "<work>").replace(a.repo.rstrip("/") + "/", "")
        self.runs = read_runs(os.path.join(w, "remote", "runs.tsv"))
        self.hc = read_kv(os.path.join(w, "out", "harness-core.env"))
        self.summary = read_kv(os.path.join(w, "out", "summary.env"))
        self.last = {}
        for row in self.ledger:
            self.last[row["gate"]] = row

    # --- facts
    def status(self):
        return self.summary.get("TERMINAL_STATE") or self.state.get("TERMINAL_STATE") or "IN_PROGRESS"

    def image(self):
        return self.state.get("CANDIDATE_IMAGE") or self.a.image or "not built yet"

    def last_run(self, name):
        found = [r for r in self.runs if r["run"] == name]
        return found[-1] if found else None

    def gate(self, name):
        row = self.last.get(name)
        return (row["result"], row["evidence"]) if row else ("NOT RUN", "")

    def env_line(self):
        env = self.fields.get("cache_env", "none")
        if env in ("", "none"):
            return "none needed; the plugin finds the cache from the workspace alone"
        key, rel = env.split("=", 1)
        status = self.hc.get("HC_STATUS", self.gate("harness-core")[0])
        where = "`%s=<workspace>/%s`" % (key, rel)
        if status == "COMMITTED":
            return "%s, injected by harness-core (branch `%s`)" % (where, self.hc.get("HC_BRANCH", "?"))
        if status == "ALREADY_INJECTED":
            return "%s, already injected by harness-core" % where
        return "%s, harness-core change %s" % (where, status.lower().replace("_", " "))

    def local_line(self):
        m = self.metrics
        if self.gate("test-public")[0] != "PASS":
            return "%s (%s)" % (self.gate("test-public")[0], self.gate("test-public")[1] or "not run")
        parts = ["cold install %s" % secs(m.get("LOCAL_COLD_INSTALL_S")), "warm install %s" % secs(m.get("LOCAL_WARM_INSTALL_S"))]
        if m.get("LOCAL_OFFLINE_S"):
            parts.append("offline check %s" % secs(m.get("LOCAL_OFFLINE_S")))
        parts.append("archive %s" % kb_to_mb(m.get("LOCAL_ARCHIVE_KB")))
        return ", ".join(parts)

    def remote_line(self, names):
        bits = []
        for name in names:
            r = self.last_run(name)
            if not r:
                continue
            extra = []
            if r["metrics"].get("install_seconds"):
                extra.append("install %s" % secs(r["metrics"]["install_seconds"]))
            label = "%s %s" % (name.replace("product-", ""), r["result"])
            bits.append(link(label, r["url"]) + (" (%s)" % ", ".join(extra) if extra else ""))
        return " · ".join(bits) if bits else "not run"

    def glance(self):
        f = self.fields
        rows = [
            ("Status", "**%s**" % self.status()),
            ("Detected by", "%s at the workspace root or one level below" % ", ".join(code(x.strip()) for x in f.get("detect_files", "").split(","))),
            ("Cache key", "MD5 of %s" % ", ".join(code(x.strip()) for x in f.get("key_inputs", "n/a").split(","))),
            ("Cached paths", ", ".join(code(x.strip()) for x in f.get("cache_paths", "").split(","))),
            ("Never cached", ", ".join(code(x.strip()) for x in f.get("excluded_paths", "").split(","))),
            ("Build-step env", self.env_line()),
            ("Local, public project", "%s @ %s: %s" % (f.get("fixture_repo", "?"), code(f.get("fixture_ref", "?")), self.local_line())),
            ("Harness, candidate image", self.remote_line(["cold", "warm", "invalidate"])),
            ("Harness, real Cache Intelligence", self.remote_line(["product-seed", "product-warm"]) if self.last_run("product-seed") or self.last_run("product-warm") else "not run (opt-in `product_path`)"),
            ("Candidate image", code(self.image())),
        ]
        out = ["| | |", "|---|---|"]
        out += ["| %s | %s |" % (k, v) for k, v in rows]
        return "\n".join(out)

    def gate_table(self, compact=False):
        out = ["| Gate | Result | Evidence |", "|------|--------|----------|"]
        for name, label in GATES:
            result, ev = self.gate(name)
            if compact and result == "NOT RUN" and name in ("test-lifecycle", "product-seed", "product-warm"):
                continue
            if compact and len(ev) > 140:
                ev = ev[:137] + "..."
            out.append("| %s | %s | %s |" % (label, result, ev.replace("|", "\\|") or "—"))
        return "\n".join(out)

    def local_section(self):
        m, f = self.metrics, self.fields
        if not m.get("LOCAL_FIXTURE_SHA"):
            return "Not run. %s" % (self.gate("test-public")[1] or "")
        rows = [
            ("Fixture", "%s @ %s (`%s`)" % (f.get("fixture_repo"), code(f.get("fixture_ref")), m.get("LOCAL_FIXTURE_SHA", "")[:12])),
            ("Runner", "%s%s" % (m.get("LOCAL_RUNNER", "?"), (" with " + code(m["LOCAL_TOOL_VERSION"])) if m.get("LOCAL_TOOL_VERSION") else "")),
            ("1. Cold restore", "miss, as expected"),
            ("2. Install (cold)", "%s with `%s`" % (secs(m.get("LOCAL_COLD_INSTALL_S")), f.get("install_cmd"))),
            ("3. Save", "%s, %s in %s archive file(s); archive scan: %s" % (secs(m.get("LOCAL_SAVE_S")), kb_to_mb(m.get("LOCAL_ARCHIVE_KB")), m.get("LOCAL_ARCHIVE_FILES", "?"), m.get("LOCAL_ARCHIVE_CHECK", "n/a"))),
            ("4. Fresh checkout", "`git clean -ffdx` removed every untracked and ignored file"),
            ("5. Warm restore", "%s, %s restored into %s" % (secs(m.get("LOCAL_WARM_RESTORE_S")), kb_to_mb(m.get("LOCAL_WARM_KB")), code(f.get("warm_check")))),
            ("6. Offline check", ("%s with `%s`" % (secs(m.get("LOCAL_OFFLINE_S")), f.get("offline_cmd"))) if m.get("LOCAL_OFFLINE_S") else "not applicable"),
            ("7. Install (warm)", secs(m.get("LOCAL_WARM_INSTALL_S"))),
            ("8. Invalidation", "changing %s gave key `%s` instead of `%s`; restore missed" % (code(f.get("invalidate_file")), m.get("LOCAL_INVALIDATED_KEY", "?"), m.get("LOCAL_KEY", "?"))),
        ]
        return "\n".join(["| Step | Result |", "|------|--------|"] + ["| %s | %s |" % r for r in rows])

    def remote_section(self):
        if not self.runs:
            return "Not run yet."
        out = ["| Run | Result | Execution | Install | Restored | Image |", "|-----|--------|-----------|---------|----------|-------|"]
        for r in self.runs:
            m = r["metrics"]
            out.append("| %s | %s | %s | %s | %s | %s |" % (
                r["run"], r["result"], link("open", r["url"]), secs(m.get("install_seconds")),
                kb_to_mb(m["cache_kb"]) if m.get("cache_kb") else m.get("restore_state", "—"), code(r["image"])))
        notes = [
            "",
            "- **cold** skips restore, so its install time is a true cold baseline; save then overwrites the key with the candidate's archive.",
            "- **warm** must restore %s before install, or the run fails." % code(self.fields.get("warm_check")),
            "- **invalidate** changes %s and must miss; it never saves." % code(self.fields.get("invalidate_file")),
        ]
        env = self.fields.get("cache_env", "none")
        if env not in ("", "none"):
            notes.append("- Every step gets %s as a stage variable, exactly what the harness-core change injects." % code(env.replace("=", "=/harness/", 1)))
        if any(r["run"].startswith("product") for r in self.runs):
            notes.append("- **product** runs use `caching.enabled: true` with the account-level Cache Intelligence image overridden to the candidate for the duration of the run, then reset.")
        return "\n".join(out + notes)

    def harness_core_section(self):
        status = self.hc.get("HC_STATUS") or self.gate("harness-core")[0]
        if status in ("NOT_APPLICABLE", "NOT RUN") and self.fields.get("cache_env", "none") in ("", "none"):
            return "Not needed: the tool's cache location is found from the workspace alone."
        lines = ["- Status: **%s**%s" % (status, (" (commit `%s`)" % self.hc["HC_COMMIT"][:12]) if self.hc.get("HC_COMMIT") else (": " + self.gate("harness-core")[1]))]
        if self.hc.get("HC_ENV"):
            env, suffix = self.hc["HC_ENV"], self.hc.get("HC_SUFFIX", "")
            lines += [
                "- Variable: `%s`; VM (Harness Cloud, self-hosted VMs): `<workDir>/%s`; Kubernetes: `/harness/%s/`; Windows: `C:\\harness\\%s\\`" % (env, suffix, suffix, suffix.replace("/", "\\")),
                "- Files: `ContainerExecutionConstants`, `VmInitializeUtilsImpl`, `K8InitializeTaskUtils`, and their unit tests (877 and 332)",
                "- Side effects: %s" % self.fields.get("cache_env_side_effects", "not recorded"),
            ]
        if self.hc.get("HC_BRANCH"):
            lines.append("- Branch: `%s` on harness-core (`develop` base); the pull request link is in this PR's description" % self.hc["HC_BRANCH"])
        lines.append("- Release order: ship the drone-cache release first, then bump the `plugins/cache` image in ci-manager config together with or after this change.")
        return "\n".join(lines)

    def ledger_table(self):
        out = ["| When (UTC) | Gate | Result | Evidence |", "|------------|------|--------|----------|"]
        for r in self.ledger:
            out.append("| %s | %s | %s | %s |" % (r["at"], r["gate"], r["result"], r["evidence"].replace("|", "\\|")))
        return "\n".join(out)

    def research_body(self):
        keep = [h for h in self.research if h not in ("Fields", "Existing support")]
        return "\n\n".join("### %s\n\n%s" % (h, demote(self.research[h])) for h in keep) or "Not recorded."

    def design_body(self):
        keep = [h for h in ("Mapping entry", "Paths", "Scenario matrix") if h in self.design]
        return "\n\n".join("### %s\n\n%s" % (h, demote(self.design[h])) for h in keep) or "Not recorded."

    def write_report(self, path):
        a, f = self.a, self.fields
        doc = [
            "# Cache Intelligence: %s" % a.tool,
            "",
            "> Generated by `cache-intel.sh report` from the worker's task state. Worker `%s`, branch `%s`, base `%s` @ `%s`." % (
                a.worker, a.branch, self.state.get("BASE_BRANCH", "?"), self.state.get("STARTING_SHA", "?")[:12]),
            "",
            "## At a glance",
            "",
            self.glance(),
            "",
            "## Summary",
            "",
            self.design.get("Summary", "Not recorded."),
            "",
            "## Validation gates",
            "",
            self.gate_table(),
            "",
            "## Local validation on a public project",
            "",
            self.local_section(),
            "",
            "## Harness validation",
            "",
            self.remote_section(),
            "",
            "## harness-core change",
            "",
            self.harness_core_section(),
            "",
            "## Research",
            "",
            "Versions verified: %s." % f.get("tool_versions", "n/a"),
            "",
            self.research_body(),
            "",
            "## Design",
            "",
            self.design_body(),
            "",
            "## Compatibility and migration",
            "",
            self.design.get("Compatibility and migration", "Not recorded."),
            "",
            "## Limitations",
            "",
            self.design.get("Limitations", "Not recorded."),
            "",
            "## Rollout and rollback",
            "",
            "Release through the normal drone-cache release. Projects that start being detected get a new composite key, so their first run after rollout is cold. "
            "Roll back by reverting the mapping entry and preparer; an image rollback alone does not reuse keys created by this change.",
            "",
            "## Command ledger",
            "",
            "<details><summary>%d entries, oldest first (failed attempts are kept)</summary>" % len(self.ledger),
            "",
            self.ledger_table(),
            "",
            "</details>",
            "",
        ]
        with open(path, "w", encoding="utf-8") as fh:
            fh.write("\n".join(doc))

    def write_pr_body(self, path):
        a = self.a
        report_url = "https://github.com/%s/blob/%s/REPORT.md" % (a.repo_name, a.branch)
        doc = [
            "## Summary",
            "",
            self.design.get("Summary", "Adds Cache Intelligence support for %s." % a.tool),
            "",
            self.glance(),
            "",
            "## Validation",
            "",
            self.gate_table(compact=True),
            "",
            "## harness-core",
            "",
            self.harness_core_section(),
            "",
            "## Limitations",
            "",
            self.design.get("Limitations", "Not recorded."),
            "",
            "Full research, design, measurements and command ledger: [REPORT.md](%s)." % report_url,
            "",
            "_Generated by the Cache Intelligence worker agent (`%s`)._" % a.worker,
            "",
        ]
        with open(path, "w", encoding="utf-8") as fh:
            fh.write("\n".join(doc))

    def write_harness_core_body(self, path):
        if not self.hc.get("HC_ENV"):
            return
        env, suffix = self.hc["HC_ENV"], self.hc.get("HC_SUFFIX", "")
        runs = "\n".join("  - %s: %s" % (r["run"], link(r["result"], r["url"])) for r in self.runs) or "  - not run yet"
        doc = [
            "## What",
            "",
            "Inject `%s` into every step of a CI stage when Cache Intelligence is enabled, like the existing pip, Poetry, uv and CocoaPods variables:" % env,
            "",
            "- VM infrastructure (Harness Cloud, self-hosted VMs): `<workDir>/%s`" % suffix,
            "- Kubernetes: `/harness/%s/`" % suffix,
            "- Windows: `C:\\harness\\%s\\`" % suffix.replace("/", "\\"),
            "",
            "## Why",
            "",
            self.fields.get("cache_env_reason", ""),
            "",
            "drone-cache (`plugins/cache`) gains %s autodetection in the companion change (branch `%s` on %s). It caches `%s`, which only lands in the workspace when this variable is set." % (
                self.a.tool, self.a.branch, self.a.repo_name, self.fields.get("cache_paths", "")),
            "",
            "## Compatibility",
            "",
            "The variable applies to every step of every stage with caching enabled. %s" % self.fields.get("cache_env_side_effects", "Not recorded."),
            "",
            "## Testing",
            "",
            "- Unit tests: `VmInitializeUtilsImplTest` and both `K8InitializeTaskUtilsTest` classes (877 and 332) now assert the new variable for Linux, macOS and Windows. The worker has no Bazel toolchain, so this PR's CI is the first run; run `make test f=<test>` locally before merging.",
            "- The injection was simulated on Harness QA0 by setting the same variable at stage level for the candidate plugin image:",
            runs,
            "",
            "## Rollout",
            "",
            "Merge together with or after the drone-cache release that contains %s support, and bump the `plugins/cache` image tag in the ci-manager config in a follow-up change." % self.a.tool,
            "",
            "_Generated by the Cache Intelligence worker agent (`%s`)._" % self.a.worker,
            "",
        ]
        with open(path, "w", encoding="utf-8") as fh:
            fh.write("\n".join(doc))


# ---------------------------------------------------------------- main

def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = p.add_subparsers(dest="cmd", required=True)
    s = sub.add_parser("fields")
    s.add_argument("file")
    s = sub.add_parser("check")
    s.add_argument("kind", choices=["research", "design"])
    s.add_argument("file")
    s = sub.add_parser("fixture-commit")
    s.add_argument("--from", dest="src", required=True)
    s.add_argument("--out", required=True)
    s.add_argument("--title", default="Cache Intelligence fixture")
    s.add_argument("--branch", default="main")
    s.add_argument("--files", default="")
    s.add_argument("--max-kb", type=int, default=256)
    s = sub.add_parser("report")
    s.add_argument("--work", required=True)
    s.add_argument("--repo", required=True)
    s.add_argument("--tool", required=True)
    s.add_argument("--worker", required=True)
    s.add_argument("--branch", required=True)
    s.add_argument("--repo-name", default="drone-plugins/drone-meltwater-cache")
    s.add_argument("--image", default="")
    a = p.parse_args()

    if a.cmd == "fields":
        for k, v in parse_fields(read(a.file)).items():
            print("%s\t%s" % (k, v))
    elif a.cmd == "check":
        text = read(a.file)
        problems = check_research(text) if a.kind == "research" else check_design(text)
        for prob in problems:
            print("- " + prob)
        sys.exit(1 if problems else 0)
    elif a.cmd == "fixture-commit":
        files = [x.strip() for x in a.files.split(",") if x.strip()]
        fixture_commit(a.src, a.out, a.title, a.branch, files, a.max_kb)
    elif a.cmd == "report":
        r = Report(a)
        r.write_report(os.path.join(a.repo, "REPORT.md"))
        out = os.path.join(a.work, "out")
        os.makedirs(out, exist_ok=True)
        r.write_pr_body(os.path.join(out, "PR_BODY.md"))
        r.write_harness_core_body(os.path.join(out, "HARNESS_CORE_PR.md"))
        print("REPORT=%s" % os.path.join(a.repo, "REPORT.md"))
        print("PR_BODY=%s" % os.path.join(out, "PR_BODY.md"))
        if os.path.exists(os.path.join(out, "HARNESS_CORE_PR.md")):
            print("HARNESS_CORE_PR_BODY=%s" % os.path.join(out, "HARNESS_CORE_PR.md"))


if __name__ == "__main__":
    main()
