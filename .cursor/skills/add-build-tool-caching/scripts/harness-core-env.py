#!/usr/bin/env python3
"""Inject a Cache Intelligence cache-directory environment variable in harness-core.

Applies the same shape of change as harness-core commit 42caa0346d5e ("add python cache
support"): a constant block in ContainerExecutionConstants, the variable in both
getCacheEnvironmentVariable implementations (VM and Kubernetes), and assertions in their
unit tests. Every edit is anchored on existing code; when an anchor is missing the script
fails without writing anything, so the change can be made by hand following that commit.

Exit codes: 0 applied, 1 error, 3 the variable is already injected (nothing to do).
"""
import argparse
import os
import re
import sys
import textwrap

CONSTANTS = "879-pipeline-ci-commons/src/main/java/io/harness/ci/commonconstants/ContainerExecutionConstants.java"
VM = "877-pipeline-ci-cd-commons/src/java/io/harness/ci/execution/integrationstage/vm/VmInitializeUtilsImpl.java"
K8 = "877-pipeline-ci-cd-commons/src/java/io/harness/ci/execution/integrationstage/k8s/K8InitializeTaskUtils.java"
VM_TEST = "877-pipeline-ci-cd-commons/src/test/java/io/harness/ci/execution/integrationstage/vm/VmInitializeUtilsImplTest.java"
K8_TEST = "877-pipeline-ci-cd-commons/src/test/java/io/harness/ci/execution/integrationstage/k8s/K8InitializeTaskUtilsTest.java"
K8_TEST_CI = "332-ci-manager/service/src/test/java/io/harness/ci/execution/integrationstage/K8InitializeTaskUtilsTest.java"
FILES = [CONSTANTS, VM, K8, VM_TEST, K8_TEST, K8_TEST_CI]
IMPORT_PREFIX = "import static io.harness.ci.commonconstants.ContainerExecutionConstants."


class AnchorError(Exception):
    pass


def java_literal(value):
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"') + '"'


def indent_of(line):
    return line[: len(line) - len(line.lstrip())]


def block_end(lines, start):
    """Index of the line that closes the brace opened on lines[start]."""
    depth = 0
    for i in range(start, len(lines)):
        depth += lines[i].count("{") - lines[i].count("}")
        if depth == 0 and i > start:
            return i
    raise AnchorError("unbalanced braces after line %d" % (start + 1))


def if_branch_end(lines, start):
    """Index of the line closing the if-branch opened on lines[start] (`}` or `} else {`)."""
    depth = 0
    for i in range(start, len(lines)):
        before = depth
        depth += lines[i].count("{") - lines[i].count("}")
        if i > start and before == 1 and lines[i].lstrip().startswith("}"):
            return i
    raise AnchorError("unterminated if-branch after line %d" % (start + 1))


def find_line(lines, pattern, start=0, end=None, what=None):
    rx = re.compile(pattern)
    for i in range(start, len(lines) if end is None else end):
        if rx.search(lines[i]):
            return i
    raise AnchorError("anchor not found: %s" % (what or pattern))


def last_line(lines, pattern, start, end, what):
    rx = re.compile(pattern)
    found = [i for i in range(start, end) if rx.search(lines[i])]
    if not found:
        raise AnchorError("anchor not found: %s" % what)
    return found[-1]


def add_static_imports(lines, names):
    for name in names:
        new = IMPORT_PREFIX + name + ";"
        if new in lines:
            continue
        static = [i for i, l in enumerate(lines) if l.startswith("import static ")]
        if not static:
            raise AnchorError("no static imports")
        first = static[0]
        last = first
        while last + 1 < len(lines) and lines[last + 1].startswith("import static "):
            last += 1
        pos = last + 1
        for i in range(first, last + 1):
            if lines[i] > new:
                pos = i
                break
        lines.insert(pos, new)


def edit_constants(lines, a):
    if any(re.search(r"\b%s_CACHE_ENV_NAME\b" % a.const, l) for l in lines):
        raise AnchorError("%s_CACHE_ENV_NAME already exists" % a.const)
    anchor = last_line(lines, r"public static final String \w+_CACHE_DIR_SUFFIX = ", 0, len(lines),
                       "last *_CACHE_DIR_SUFFIX constant")
    ind = indent_of(lines[anchor])
    comment = textwrap.wrap(a.comment, width=100 - len(ind) - 3) if a.comment else []
    block = [ind + "// " + c for c in comment]
    block.append('%spublic static final String %s_CACHE_ENV_NAME = %s;' % (ind, a.const, java_literal(a.env)))
    block.append('%spublic static final String %s_CACHE_DIR = %s;' % (ind, a.const, java_literal(a.linux_dir + "/")))
    if a.windows:
        block.append('%spublic static final String %s_CACHE_DIR_WIN = %s;' % (ind, a.const, java_literal(a.win_dir + "\\")))
    block.append('%spublic static final String %s_CACHE_DIR_SUFFIX = %s;' % (ind, a.const, java_literal("/" + a.suffix)))
    lines[anchor + 1:anchor + 1] = block


def edit_cache_method(lines, a, signature, windows_cond, linux_value):
    start = find_line(lines, re.escape(signature), what=signature)
    end = block_end(lines, start)
    win_start = find_line(lines, re.escape(windows_cond), start, end, windows_cond)
    win_end = if_branch_end(lines, win_start)
    if a.windows:
        ref = last_line(lines, r"envVars\.put\(", win_start, win_end, "Windows envVars.put")
        lines.insert(ref + 1, "%senvVars.put(%s_CACHE_ENV_NAME, %s_CACHE_DIR_WIN);" % (indent_of(lines[ref]), a.const, a.const))
        win_end += 1
        end += 1
    ref = last_line(lines, r"envVars\.put\(", win_end, end, "Linux envVars.put")
    lines.insert(ref + 1, "%senvVars.put(%s_CACHE_ENV_NAME, %s);" % (indent_of(lines[ref]), a.const, linux_value))


def edit_test_method(lines, method, env, value):
    start = find_line(lines, r"public void %s\(\)" % re.escape(method), what=method)
    end = block_end(lines, start)
    ref = last_line(lines, r"containsEntry\(", start, end, "containsEntry in " + method)
    lines.insert(ref + 1, "%sassertThat(envVars).containsEntry(%s, %s);" % (indent_of(lines[ref]), java_literal(env), java_literal(value)))


def plan(a):
    edits = {}

    def constants(lines):
        edit_constants(lines, a)

    def vm(lines):
        names = ["%s_CACHE_DIR_SUFFIX" % a.const, "%s_CACHE_ENV_NAME" % a.const]
        if a.windows:
            names.append("%s_CACHE_DIR_WIN" % a.const)
        add_static_imports(lines, names)
        edit_cache_method(lines, a, "public Map<String, String> getCacheEnvironmentVariable(String fullWorkDir, OSType os) {",
                          "if (os == OSType.Windows) {", "fullWorkDir + %s_CACHE_DIR_SUFFIX" % a.const)

    def k8(lines):
        names = ["%s_CACHE_DIR" % a.const, "%s_CACHE_ENV_NAME" % a.const]
        if a.windows:
            names.append("%s_CACHE_DIR_WIN" % a.const)
        add_static_imports(lines, names)
        edit_cache_method(lines, a, "public Map<String, String> getCacheEnvironmentVariable(OSType osType) {",
                          "if (osType == OSType.Windows) {", "%s_CACHE_DIR" % a.const)

    def vm_test(lines):
        edit_test_method(lines, "testGetCacheEnvironmentVariableForLinux", a.env, a.linux_dir)
        edit_test_method(lines, "testGetCacheEnvironmentVariableIsRelativeToWorkDirOnMacOS", a.env, "/tmp" + a.linux_dir)
        if a.windows:
            edit_test_method(lines, "testGetCacheEnvironmentVariableForWindows", a.env, a.win_dir + "\\")

    def k8_test(lines):
        edit_test_method(lines, "testGetCacheEnvironmentVariableForLinux", a.env, a.linux_dir + "/")
        edit_test_method(lines, "testGetCacheEnvironmentVariableForMacOS", a.env, a.linux_dir + "/")
        if a.windows:
            edit_test_method(lines, "testGetCacheEnvironmentVariableForWindows", a.env, a.win_dir + "\\")

    edits[CONSTANTS] = constants
    edits[VM] = vm
    edits[K8] = k8
    edits[VM_TEST] = vm_test
    edits[K8_TEST] = k8_test
    edits[K8_TEST_CI] = k8_test
    return edits


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--repo", required=True, help="harness-core checkout")
    p.add_argument("--env", required=True, help="environment variable, e.g. CARGO_HOME")
    p.add_argument("--suffix", required=True, help="workspace-relative directory, e.g. .cache/cargo")
    p.add_argument("--const", required=True, help="constant prefix, e.g. CARGO")
    p.add_argument("--comment", default="", help="why the variable must be injected (code comment)")
    p.add_argument("--no-windows", dest="windows", action="store_false", help="Linux and macOS only")
    p.add_argument("--check", action="store_true", help="only report whether the variable is already injected")
    a = p.parse_args()

    if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", a.env):
        sys.exit("invalid --env %r" % a.env)
    if not re.fullmatch(r"[A-Z][A-Z0-9_]*", a.const):
        sys.exit("invalid --const %r" % a.const)
    a.suffix = a.suffix.strip().strip("/")
    if not a.suffix or a.suffix.startswith("..") or os.path.isabs(a.suffix):
        sys.exit("--suffix must be a workspace-relative directory")
    a.linux_dir = "/harness/" + a.suffix
    a.win_dir = "C:\\harness\\" + a.suffix.replace("/", "\\")

    contents = {}
    for rel in FILES:
        path = os.path.join(a.repo, rel)
        if not os.path.isfile(path):
            sys.exit("missing %s (sparse checkout must include it)" % rel)
        with open(path, encoding="utf-8") as f:
            contents[rel] = f.read()

    if re.search(r'_CACHE_ENV_NAME = %s;' % re.escape(java_literal(a.env)), contents[CONSTANTS]):
        print("ALREADY_INJECTED %s (ContainerExecutionConstants already defines it)" % a.env)
        sys.exit(3)
    if a.check:
        print("NOT_INJECTED %s" % a.env)
        return

    updated = {}
    try:
        for rel, edit in plan(a).items():
            lines = contents[rel].split("\n")
            edit(lines)
            updated[rel] = "\n".join(lines)
    except AnchorError as e:
        sys.exit("harness-core layout changed, apply by hand following commit 42caa0346d5e: %s" % e)

    for rel, text in updated.items():
        with open(os.path.join(a.repo, rel), "w", encoding="utf-8") as f:
            f.write(text)
        print("UPDATED %s" % rel)
    print("INJECTED %s VM=<workDir>/%s K8s=%s/ Windows=%s" % (a.env, a.suffix, a.linux_dir, a.win_dir + "\\" if a.windows else "skipped"))


if __name__ == "__main__":
    main()
