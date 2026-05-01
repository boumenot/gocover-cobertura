## Forked from t-yuki

This is a **fork** of https://github.com/boumenot/gocover-cobertura.

At the time of this writing the repository appears to be on *pause* with
several outstanding PRs, and forks with interesting contributions.  This
repo consolidates those outstanding forks, and combines them into one repo.

\[**2026-05-01**\]: This project has evolved from a private fork
created for my own needs into something that is now used by many
others. Historically, I have not focused heavily on maintaining this
repository—I’ve occasionally merged PRs or made fixes, but for my
purposes, the functionality was “good enough.”

However, that context has changed. While this fork met my original
goals, it is now clear that others are relying on it. As a result, I
feel a greater sense of responsibility for its quality and stability.

To address this, I plan to strengthen the repository by introducing a
regression test suite based on popular open-source Go projects. The
current test coverage is minimal and needs improvement. Although
several PRs have identified issues, the lack of comprehensive testing
makes it difficult to confidently ensure that new changes do not
introduce regressions.

The goal of the regression suite is to lock in current behavior and
enable more deliberate, explicit decisions when making changes. I also
hope to extract simplified reproductions from identified issues, which
should help improve long-term maintainability (AI has been especially
helpful in this area).

I don’t expect to spend much more time here, but I will be more
intentional about stewardship now that others depend on it.

go tool cover XML (Cobertura) export
====================================

This is a simple helper tool for generating XML output in [Cobertura](http://cobertura.sourceforge.net/) format
for CIs like [Jenkins](https://wiki.jenkins-ci.org/display/JENKINS/Cobertura+Plugin) and others
from [go tool cover](https://code.google.com/p/go.tools/) output.

Installation
------------

Just type the following to install the program and its dependencies:

    $ go get github.com/boumenot/gocover-cobertura

Usage
-----

`gocover-cobertura` reads from the standard input:

    $ go test -coverprofile=coverage.txt -covermode count github.com/gorilla/mux
    $ gocover-cobertura < coverage.txt > coverage.xml

Note that you should run this from the directory which holds your `go.mod` file.

Some flags can be passed (each flag should only be used once):

- `-by-files`

  Code coverage is organized by class by default.  This flag organizes code
  coverage by the name of the file, which the same behavior as `go tool cover`.

- `-ignore-dirs PATTERN`

  ignore directories matching `PATTERN` regular expression. Full
  directory names are matched, as
  `github.com/boumenot/gocover-cobertura` (and so `github.com/boumenot`
  and `github.com`), examples of use:
  ```
  # A specific directory
  -ignore-dirs '^github\.com/boumenot/gocover-cobertura/testdata$'
  # All directories autogen and any of their subdirs
  -ignore-dirs '/autogen$'
  ```

- `-ignore-files PATTERN`

  ignore files matching `PATTERN` regular expression. Full file names
  are matched, as `github.com/boumenot/gocover-cobertura/profile.go`,
  examples of use:
  ```
  # A specific file
  -ignore-files '^github\.com/boumenot/gocover-cobertura/profile\.go$'
  # All files ending with _gen.go
  -ignore-files '_gen\.go$'
  # All files in a directory autogen (or any of its subdirs)
  -ignore-files '/autogen/'
  ```

- `-ignore-gen-files`

  ignore generated files. Typically files containing a comment
  indicating that the file has been automatically generated. See
  `genCodeRe` regexp in [ignore.go](ignore.go).

Troubleshooting
---------------

*Error*: code coverage conversion failed: unable to determine file path for <file>

If you encounter this error, it may be due to missing build tags when running `gocover-cobertura`. You need to run the tool with the same tags you used when running the tests.
To pass the tags, set the GOFLAGS environment variable:

    GOFLAGS="-tags=<tags from go test>" gocover-cobertura < coverage.txt > coverage.xml

Regression Tests
----------------

A snapshot-based regression test suite verifies that conversion output remains
stable across code changes.  The suite converts real coverage profiles from
open-source Go projects and compares the XML output against checked-in golden
files.

**Running regression tests** (Linux only):

    go test -tags regression -run TestRegression -v -timeout 600s

**Updating golden files** after intentional changes:

    go test -tags regression -run TestRegression -update -v -timeout 600s
    git diff testdata/regression/  # review the changes

**Platform note:** Golden files are generated on Linux.  The regression tests
are skipped on non-Linux platforms because coverage profiles may reference
platform-specific source files (e.g. `_windows.go`) that are unavailable on
other operating systems.  To regenerate golden files on Windows, use WSL.

**Current corpus:**

| Project | Tag | Lines |
|---------|-----|-------|
| gorilla/mux | v1.8.1 | 946 |
| spf13/cobra | v1.8.1 | 4,420 |
| approvals/go-approval-tests | v1.9.1 | 1,785 |
| junegunn/fzf | v0.60.0 | 14,137 |
| stretchr/testify | v1.10.0 | 3,196 |
| prometheus/prometheus | v3.2.0 | 78,123 |
| kubernetes/kubernetes | v1.32.0 | 26,956 |

~~Authors~~Merger
-------

[Christopher Boumenot (boumenot)](https://github.com/boumenot)

Thanks
------

 * [Yukinari Toyota (t-yuki)](https://github.com/t-yuki)
 * This tool is originated from [gocov-xml](https://github.com/AlekSi/gocov-xml) by [Alexey Palazhchenko (AlekSi)](https://github.com/AlekSi)
 * [DarcySail](https://github.com/DarcySail)'s [PR](https://github.com/t-yuki/gocover-cobertura/pull/22)
 * [maxatome](https://github.com/maxatome)'s [PR](https://github.com/t-yuki/gocover-cobertura/pull/19)
 * [elliotmr](https://github.com/elliotmr)'s [branch](https://github.com/elliotmr/gocover-cobertura)
