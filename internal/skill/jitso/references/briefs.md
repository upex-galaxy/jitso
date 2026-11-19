# Writing a brief

A brief is the whole job description of a worker. The worker has none of your conversation, so write it for a reader who knows nothing but the repo. It travels as a file, never as an argument, and it goes into `.jitso/briefs/<name>.md`: a brief anywhere else is `JITSO REJECTED: brief file is outside the allowed folders`. The launcher puts the card on top of it (and, when the build lists skills, a `Load first:` line), so do not copy the card.

## What the launcher checks

`jitso launch` runs four checks and reports every one that fails, in this order (the code is `internal/brief/check.go` in the jitso source; a consumer project only has the messages, so read them):

| Check | Passes when |
|---|---|
| `sections` | the text has a heading line for each of `Goal`, `Scope`, `Done when` and `Report format`. Any heading level, any case, an optional trailing colon, nothing else on the line |
| `length` | the word count is inside the limits the rejection prints. A word is a run of characters between spaces that holds a letter: `` `src/sync.ts` `` and `don't` are one word each, a number such as `3.14` is none, and the heading lines (`## Goal` and the like) are not counted |
| `language` | the text reads as English. The check counts common English and Spanish words, so a short brief of identifiers and paths can fail it. `--brief-language es` or `any` skips it, only for a project that works in another language |
| `paths` | every backticked token that looks like a path exists |

A backticked token counts as a path when it contains a slash and does not start with `~` or `$`, has no URL scheme and none of `*`, `<`, `>`, `{`, `}`. A relative path is looked up from the project's primary checkout. So `docs/guide.md` must exist, and so must something like `and/or` if you backtick it: leave plain words out of backticks.

A file the worker will create is exempt when its path is on a line that starts with `Output:`, `Outputs:`, `Create:` or `Creates:` (any case, an optional list dash first). Put new files there:

```
Output: `src/new-module.ts`
```

Real rejections, from a scratch project (the limits come from the launcher, so quote them from your own run):

```
JITSO REJECTED: brief failed: sections (missing section: Scope, Done when, Report format); length (3 words, expected 40 to 800)
JITSO REJECTED: brief failed: paths (cited path does not exist: docs/missing-guide.md). A file the worker will create goes on a line that starts with Output: or Creates:
JITSO REJECTED: brief failed: language (the brief must be written in English)
```

Fix what the message names and run `--dry-run` again. Do not pad a brief to pass `length`: a short brief that says what to do and how to know it is done is better than a long one.

## What a good brief holds

- **Goal**: one outcome in a sentence or two, with the ticket or story key if there is one.
- **Scope**: what to touch, and what not to. Name folders and files the worker needs to read. Anything the brief does not ask for is out of bounds.
- **Done when**: evidence the worker can produce, such as a test that passes or a file read back from disk. A success code from a tool is not evidence.
- **Report format**: what to send back. Ask for a count (done, failed, open) and the paths touched, not a transcript. In fleet mode the report goes to the launcher, so say what the launcher needs to decide the next step.

Also:

- Name credentials by variable, never by value. Do not paste a secret into a brief; the brief is stored in the launch folder and its hash in the registry.
- State assumptions you made and decisions already taken, so the worker does not reopen them.
- One job per brief. Two jobs are two launches.
- Do not tell a worker to start other sessions, push, publish or message a person unless the human asked for that.

## Pointing at a card you already have

When the job is already written in another format (for example a fleet card another skill wrote inside the project), do not rewrite it. Make the brief file under `.jitso/briefs/` hold one line and nothing else:

```
Brief: docs/cards/worker-1.md
```

The keyword may be in any case and the path may sit in backticks. A second non-blank line makes the file an ordinary brief again, checked as above. The launcher then:

- resolves the path from the project's primary checkout (not from your working folder), follows every symlink, and requires a regular file inside the primary checkout whose real path ends in `.md`. A link that leads to `.env` or any other file that is not `.md` is refused, and so is any target (as written or after links) with a path segment `.auth`, `.jitso` or one starting with `.env`, in any case: `brief target is a protected path: <path>`;
- checks the card's text for the language (`--brief-language`) and for at least the minimum words. It asks for no sections, no maximum and no existing cited paths;
- copies the card's text into `brief.md` where a brief goes, then a `## Fields` block with the launch's `--field` values (one `- <key>: <value>` line per field, sorted; none without fields), with the JITSO rules last. The registry hash `brief_sha256` is the hash of that card text.

The pointer line carries no extra text: put any instruction the worker needs in the card itself. The target must be a file the worker may read, so keep secrets out of the card.

Rejections, each naming the path as you wrote it:

| Message | Fix |
|---|---|
| `brief target path is empty or contains a NUL byte` | write a path after `Brief:` |
| `brief target does not exist: <path>` | the path does not resolve from the primary checkout |
| `brief target is not a regular file: <path>` | name a file, not a folder |
| `brief target is outside the project: <path>` | move the card inside the primary checkout (a linked worktree does not count) |
| `brief target must be a .md file: <path>` | the real path must end in `.md` |
| `brief target is a protected path: <path>` | the path, as written or after links, has a segment `.auth`, `.jitso` or one starting with `.env`; copy the card to another folder |
| `brief target failed: length (<n> words, expected at least <minimum>)`, `language (...)` | the card is too short or not in English; fix it or pass `--brief-language` |

## A filled example

This block is verified: it passed `jitso launch --dry-run` against a fixture project that has `src/sync.ts` and `docs/guide.md`. Its paths are fixture paths, so replace them with real ones from your project.

```markdown
## Goal
Make the sync flag in `src/sync.ts` read its value from an environment variable, so a test can turn syncing off without editing source. This is for the story PROJ-12.

## Scope
Touch only `src/sync.ts` and the test that covers it. Read `docs/guide.md` for the naming conventions before you start. Do not change anything under `jitso/` and do not add a dependency. If the change needs more than this, stop and say so in your report.

## Done when
The flag still defaults to its current value when the variable is unset, the new test passes once locally, and you have read the changed file back from disk to confirm what is there.
Output: `src/sync.test.ts`

## Report format
Reply with three counts: what was done, what failed, what is open. List every file you created or changed, one path per line, and quote the exact message of any failure. Keep it under one page.
```

To prove your own brief the same way, write it to `.jitso/briefs/<name>.md` and run the dry run:

```
jitso launch --build <id> --brief .jitso/briefs/<name>.md --mode direct --dry-run
```

A card and `dry run: nothing was written and Orca was not called` mean the brief passed.

## Briefs in a fleet

A conductor writes one brief per worker. Keep each brief self-contained and put the shared facts (run, task, the files a worker owns) in every one. A fleet worker cannot ask the human, so a brief for it must say what to do when it is blocked: report the blocker to the launcher and stop.
