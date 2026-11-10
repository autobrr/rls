# Description

<!--- Summarize the change and the motivation. Link any related issue or discussion.
      For parsing changes, list example release names with the before and after values. --->

Fixes # (issue)

## Type of change

<!--- Delete options that are not relevant. --->

- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Tag info update (new or changed rows in `taginfo/taginfo.csv`)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Refactor / maintenance (no functional change)

## How has this been tested?

<!--- Describe how you verified your change. Parsing changes should add release names
      with their expected values to `tests.yaml`, and `go test ./...` should pass. --->

*

## Checklist

- [ ] I ran `go fmt` and the test suite passes locally
- [ ] I added cases to `tests.yaml` for any parsing change
- [ ] If this adds or renames a normalized value (source, codec, HDR, etc.), I noted the downstream impact on autobrr/qui
- [ ] I have linked any related issue

## AI disclosure

<!--- Please do not lie about the AI usage in this PR. --->

Was any AI used in this PR? Roughly how much of the code was AI-written, and what model/tool?
