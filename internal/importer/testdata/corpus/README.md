# The import corpus

Real ReGa databases, each with what the import makes of it. `TestCorpusFiles` reads every folder: the
database must parse and import into valid files, the CCU's own trigger table must agree with the triggers
the import finds, and the report, the programs, the variables and the values must equal the golden files
beside it (`go test ./internal/importer -run CorpusFiles -update` rewrites them - read the diff).

| Folder | What it is |
| --- | --- |
| `regahss-test` | The database of [OpenCCU/ReGaHss-Test](https://github.com/OpenCCU/ReGaHss-Test) (commit `85dcb94`, MIT, © Sebastian Raff): 28 synthetic programs on variables, keys, devices and time modules. Unchanged. |
| `ccu3-home` | A CCU3 (firmware 3.87.6) in daily use: 110 devices on BidCos-RF, BidCos-Wired, HmIP-RF and groups, 16 programs (eQ-3's counter programs among them, two with scripts), 31 variables. **Anonymised.** Not in the published repository; the test reads the folders that are there. |
| `openccu-lab` | An OpenCCU 3.89.11 with a handful of devices and no program of its own: the built-ins, a leftover of the editor. **Anonymised.** |

More databases of the same kind are in `internal/harness/testdata/*/homematic.regadom.gz`: what a real
ReGaHss saved for each scenario of the comparison (`TestHarnessDatabases`).

## Adding a database

Nobody's database goes into the repository as it was. Run it through the anonymiser first:

    go run ./cmd/logic anonymise backup.sbk /tmp/anonymous.regadom
    mkdir internal/importer/testdata/corpus/<name>
    gzip -9 -n -c /tmp/anonymous.regadom > internal/importer/testdata/corpus/<name>/homematic.regadom.gz
    go test ./internal/importer -run CorpusFiles -update

The test refuses a file the anonymiser would still change (only the folders named in `publicCorpus` are
exempt), and checks that the anonymised database imports exactly as it did before. With the original at
hand, `REGADOM_CORPUS=<folder with .sbk and .regadom files> go test ./internal/importer -run 'TestCorpus$' -v`
compares the two. What the anonymiser keeps and what it replaces: the guide, "Passing a database on".
