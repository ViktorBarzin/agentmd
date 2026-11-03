# Test redundancy in f1-stream and tripit, measured

2026-09-24. Viktor asked whether the two suites carry more tests than they
need, and chose to measure before deleting anything. This is the measurement.
Nothing was deleted.

## Method

- Clean exports of each repo's master (f1-stream `1fe28d5`, tripit `d7a0370`)
  ran under `pytest-cov --cov-branch --cov-context=test`, which tags every
  branch arc with the tests that executed it.
- A test's footprint is the set of arcs it ran during setup, call and
  teardown, inside `backend/` for f1-stream and `tripit_api/` for tripit.
  Parametrized cases are merged into their test function.
- The analysis is [`scripts/test-redundancy/redundancy.py`](https://github.com/ViktorBarzin/infra/blob/master/scripts/test-redundancy/redundancy.py) in the infra repo;
  its docstring has the commands to repeat the run.

## What coverage can and cannot tell us

Coverage records which code ran, not what a test asserted. tripit's largest
group shows the gap: 34 tests across many files, each checking that a
different endpoint answers 401 without a token. The auth dependency rejects
the request before the endpoint runs, so all 34 share one footprint, yet each
one guards a different route. Code outside the measured package is invisible
too: a test of a script under `scripts/` that calls one `backend/` helper
looks identical to every other test that calls that helper. Read the numbers
below as an upper bound on redundancy and as a list to review.

## Results

| | f1-stream | tripit |
|---|---|---|
| test functions with coverage in the measured package | 3,129 | 3,928 |
| run no branch that another function doesn't | 2,623 (84%) | 3,086 (79%) |
| footprint identical to another function's | 1,054 (34%), in 495 groups | 875 (22%), in 463 groups |
| of those groups, inside a single test file | 463 | 401 |
| fewest functions that still run every branch the suite runs | 786 (25%) | 1,198 (30%) |
| suite time on a GitHub runner | 35 s | 7 min 15 s, no longer run in CI |

229 f1-stream tests and about 250 tripit tests exercise code outside the
measured package (scripts, static assets, the CLI) and are not counted here.

## What the large groups are

Most large groups are one code path checked for many different rules:

- f1-stream: 32 tests whose only contact with `backend/` is `normalise()`.
  13 check one normalisation rule each; the other 19 test
  `scripts/mirror_logos.py`, whose own code this run does not measure. 20 tests across
  six `test_metrics_*.py` files, each rendering the metrics page and checking a
  different part of it. 15 tests across four files, each validating one
  property of the shipped channel catalogue.
- tripit: the 34 auth tests above. 30 tests across several files, each
  checking one setting's default or how it loads from the environment. 16
  tests, each checking that one provider builder defaults to its fake.

So most of what these numbers point at is consolidation: one parametrized
table per family of rules, and fewer files per subject. That keeps every
check while cutting the code agents read and maintain. Genuine duplicates are
most likely in the groups that span files (32 in f1-stream, 62 in tripit),
and each one needs both tests read before one is removed.

## Where to start

Files with the most functions whose footprint matches another function's:

| f1-stream | twins / functions | tripit | twins / functions |
|---|---|---|---|
| `test_channel_logos.py` | 52 / 84 | `test_researcher.py` | 21 / 33 |
| `test_channel_catalogue.py` | 49 / 79 | `test_trip_photos_api.py` | 20 / 64 |
| `test_transcode.py` | 38 / 66 | `test_lodging_playwright.py` | 16 / 39 |
| `test_replay_ladder.py` | 35 / 57 | `test_tour_config.py` | 16 / 17 |
| `test_share.py` | 34 / 69 | `test_watchers_api.py` | 15 / 49 |

A reasonable first pass: take one file from the top of each column, fold its
identical groups into parametrized tests, and rerun the script to see the
counts move.

## Runtime is no longer the cost

Once f1-stream's tests stopped scraping live provider sites at app boot
(f1-stream `4cdc1e1`), all 4,334 run in 35 s on a GitHub runner, and tripit's
backend suite now runs only in the local land gate. Consolidating would save
reading and maintenance time rather than CI minutes.

## Open questions

- Coverage cannot separate "same path, different assertion" from "same path,
  same assertion". Only reading a group settles which it is.
- These numbers are a snapshot. f1-stream's suite grew from 3,461 to 4,334
  tests between 17 and 21 September, so a rerun in a month will differ.
