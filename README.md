## Forked from [Christopher Boumenot (boumenot)](https://github.com/boumenot)

At the time of this writing the repository appears to be on *pause* with
several outstanding PRs, issues and forks with interesting contributions.

This repo aims to add improvements so that it can be used by our services.

go tool cover XML (Cobertura) export
====================================

This is a simple helper tool for generating XML output in [Cobertura](http://cobertura.sourceforge.net/) format
for CIs like [Jenkins](https://wiki.jenkins-ci.org/display/JENKINS/Cobertura+Plugin) and others
from [go tool cover](https://code.google.com/p/go.tools/) output.

Installation
------------

Just type the following to install the program and its dependencies:

    $ go get github.com/bountylabs/gocover-cobertura

Usage
-----

`gocover-cobertura` reads from the standard input:

    $ go test -coverprofile=coverage.out -covermode count ./
    $ gocover-cobertura < coverage.out > coverage.xml
    
Note that you should run this from the directory which holds your `go.mod` file.

Some flags can be passed (each flag should only be used once):

- `-by-files`

  Code coverage is organized by class by default.  This flag organizes code
  coverage by the name of the file, which the same behavior as `go tool cover`.

- `-ignore-dirs PATTERN`

  ignore directories matching `PATTERN` regular expression. Full
  directory names are matched, as
  `github.com/bountylabs /gocover-cobertura` (and so `github.com/bountylabs`
  and `github.com`), examples of use:
  ```
  # A specific directory
  -ignore-dirs '^github\.com/bountylabs/gocover-cobertura/testdata$'
  # All directories autogen and any of their subdirs
  -ignore-dirs '/autogen$'
  ```

- `-ignore-files PATTERN`

  ignore files matching `PATTERN` regular expression. Full file names
  are matched, as `github.com/bountylabs/gocover-cobertura/profile.go`,
  examples of use:
  ```
  # A specific file
  -ignore-files '^github\.com/bountylabs/gocover-cobertura/profile\.go$'
  # All files ending with _gen.go
  -ignore-files '_gen\.go$'
  # All files in a directory autogen (or any of its subdirs)
  -ignore-files '/autogen/'
  ```

- `-ignore-gen-files`

  ignore generated files. Typically files containing a comment
  indicating that the file has been automatically generated. See
  `genCodeRe` regexp in [ignore.go](ignore.go).

  
Thanks
------

 * [Yukinari Toyota (t-yuki)](https://github.com/t-yuki)
 * This tool is originated from [gocov-xml](https://github.com/AlekSi/gocov-xml) by [Alexey Palazhchenko (AlekSi)](https://github.com/AlekSi)
 * [DarcySail](https://github.com/DarcySail)'s [PR](https://github.com/t-yuki/gocover-cobertura/pull/22)
 * [maxatome](https://github.com/maxatome)'s [PR](https://github.com/t-yuki/gocover-cobertura/pull/19)
 * [elliotmr](https://github.com/elliotmr)'s [branch](https://github.com/elliotmr/gocover-cobertura)
