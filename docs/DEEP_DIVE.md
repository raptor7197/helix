# vit deep dive

a thorough walkthrough of what vit is, why it is shaped the way it is, and how
every piece fits together. this document uses lowercase throughout as a
stylistic choice; identifiers in code font keep their real casing because they
are code.

---

## contents

1. overview
2. the problem it solves
3. product philosophy
4. system architecture
5. the data model: domain-split json
6. json byte parity
7. the go core, module by module
8. the python bridge
9. the resolve plugin
10. the merge model in depth
11. command flows
12. testing strategy
13. build and install
14. distribution note

---

## 1. overview

vit is version control for video editing. it treats an edit the way git treats
a codebase: branches, commits, diffs, and merges. the difference is what gets
versioned. vit does not store media. it stores the edit decisions, where clips
sit on the timeline, how they are trimmed, how they are graded, how the audio is
mixed, as small json files, and it uses the real git binary as the backend.

the result is that an editor, a colorist, and a sound designer can each work on
their own branch at the same time, then merge their work together, without
copying multi-gigabyte video files around and without overwriting each other.

---

## 2. the problem it solves

the traditional post-production pipeline is sequential. the editor finishes a
cut, hands the project to the colorist, who hands it to the sound designer. each
handoff is a bottleneck. there is no clean way to work in parallel, no
structured history of who changed what, and no way to merge two people's
creative work.

the naive fix, "git for video files", does not work. media is enormous and
binary, so git cannot diff or merge it, and storing it in git is impractical.
the insight behind vit is that the thing worth versioning is not the media, it
is the metadata: the timeline decisions. those are small, structured, and
mergeable.

---

## 3. product philosophy

- metadata, not media. timeline decisions are the merge surface. media stays on
  disk and is referenced by path and checksum in a manifest.
- use git, do not reimplement it. every operation goes through the system git
  binary. vit is a policy and serialization layer on top of git, not a new
  version control system.
- domain-split json. cuts, color, audio, effects, and markers live in separate
  files so different roles edit different files and merges stay clean.
- snapshot based. each commit is a complete timeline state, not a patch.
- no database, no media storage. json in git is the entire persistence model.
- the command line is the substrate; the resolve panel is the primary user
  interface built on top of it.

---

## 4. system architecture

vit has a single source of truth for all logic: a go binary with no third-party
dependencies. everything that is not strictly tied to davinci resolve lives in
that binary.

there are two ways logic is invoked:

- the command line. a user (or a script) runs `vit <command>`. this talks
  directly to the go core.
- the resolve panel. an editor clicks buttons in a panel inside davinci resolve.
  the panel is python, because resolve embeds its own python interpreter and
  exposes no other scripting language. the panel calls thin python shims, and
  those shims shell out to the same go binary using a small json protocol.

the flow looks like this:

    resolve panel  ->  python shims  ->  vit binary (go)  ->  system git
    cli command    ->  vit binary (go)  ->  system git

the python side keeps only what must touch the resolve api directly: the
serializer (which reads a live timeline into json) and the deserializer (which
rebuilds a timeline from json). every other python module, `core.py`,
`differ.py`, `validator.py`, `merge_utils.py`, and `ai_merge.py`, is a thin shim
that forwards to the go binary. this means the resolve plugin scripts never had
to change; their imports and function signatures are identical to before.

both paths end at the same git repository, and both produce byte-identical json,
so it does not matter which side writes a file.

---

## 5. the data model: domain-split json

a vit-managed project is a git repository with this shape:

    my-video-project/
      .git/
      .vit/config.json
      timeline/
        cuts.json
        color.json
        audio.json
        effects.json
        markers.json
        metadata.json
      assets/
        manifest.json

each timeline file owns one domain:

- `cuts.json` holds the video tracks and the clips on them: id, name, media
  reference, record in and out frames, source in and out frames, track index,
  transform (pan, tilt, zoom, opacity, rotation, crop, flip, and so on), speed
  and retime settings, composite mode, and title or generator properties.
- `color.json` holds a grade per clip, keyed by the clip id. a grade is a set of
  nodes, and each node carries cdl values (slope, offset, power), primary color
  wheels (lift, gamma, gain, offset), contrast, pivot, hue, saturation, white
  balance, sharpness, and noise reduction, plus optional references to a drx
  still or a lut file.
- `audio.json` holds the audio tracks and clips: id, media reference, in and out
  frames, volume, pan, and speed.
- `effects.json` holds per-clip effects and transitions, keyed by clip id.
- `markers.json` holds the timeline markers: frame, color, name, note, duration.
- `metadata.json` holds the timeline-wide settings: project and timeline names,
  frame rate, resolution, start timecode, and track counts.

clips are linked across files by their id. a grade in `color.json` refers to the
same id as the clip in `cuts.json`. this is what makes cross-domain validation
possible: if a clip id disappears from `cuts.json` but a grade still refers to
it, that is an orphaned reference.

the split is deliberate. an editor changes `cuts.json`, a colorist changes
`color.json`, a sound designer changes `audio.json`. because these are separate
files, git merges them without conflict. the domains only interact through clip
ids, and that interaction is exactly what the validation and merge layers watch.

---

## 6. json byte parity

vit is bilingual: both the go binary and the python serializer can write the
same timeline files. if the two sides formatted json even slightly differently,
every save would produce a noisy git diff and merges would thrash.

to prevent that, the go json writer reproduces python's
`json.dump(data, indent=2, sort_keys=True)` exactly:

- two-space indentation.
- keys sorted lexicographically.
- non-ascii characters escaped as `\uxxxx`, matching python's default
  `ensure_ascii=true`, including surrogate pairs for characters above the basic
  multilingual plane.
- a trailing newline at the end of the file.
- float literals preserved. this is the subtle one. a value written as `24.0`
  must round-trip as `24.0`, not `24`. the go decoder keeps numbers as
  `json.Number` (their original text) rather than converting to float64 and
  back, so a read-modify-write cycle never changes a number's textual form.

this parity is covered by a round-trip test: a file written the python way is
read by the go layer and written back, and the bytes must be identical.

---

## 7. the go core, module by module

the go core lives in `internal/vit/`, with the command line in `cmd/vit/`.

- `jsonio.go` is the foundation. it decodes json keeping numbers as text,
  encodes with python parity, reads and writes the domain files, and provides
  the value helpers the rest of the core shares, including a numeric-aware deep
  equality that matches python's comparison semantics (so that `1` and `1.0`
  compare equal, and `true` equals `1`).
- `git.go` is the git wrapper. it shells out to the system git binary for init,
  add, commit, branch, checkout, merge, diff, log, status, revert, push, pull,
  clone, remote, and config. it also parses richer views: a log with per-commit
  file lists, and a log with parent hashes for drawing a commit graph. errors
  are surfaced as a `GitError` whose message preserves git's own text, because
  the python plugin string-matches phrases like "nothing to commit".
- `differ.go` produces the human-readable timeline diff. instead of a raw text
  diff, it reports changes in editing terms: clips added, removed, trimmed, or
  moved; transforms changed; speed and retime changes; color node changes with
  each value formatted the way an editor expects; markers and metadata changes.
  it also produces change summaries grouped by category (video, audio, color)
  used by the panel.
- `validator.go` is the post-merge validation engine. it detects orphaned
  references (a grade or effect pointing at a deleted clip), overlapping clips on
  a track, audio and video that have drifted out of sync, track-count
  mismatches, and speed and duration inconsistencies. each finding is an issue
  with a severity and a category.
- `mergeutils.go` is the overlay-aware merge policy. this is the heart of vit's
  conflict handling. when two branches collide on the same clip id in a way git
  cannot reconcile, for example one branch turned a media clip into a title, this
  module decides how to keep both: it promotes one into an overlay on a higher
  track, generates new ids, and rewrites the associated generator and grade
  sidecar file names so nothing is lost.
- `aimerge.go` is the ai layer. it talks to the gemini rest api directly (no
  sdk). it can analyze a merge across base, ours, and theirs and return
  structured per-domain decisions with confidence levels; walk a user through
  ambiguous choices; and provide enrichment such as commit-message suggestions,
  log summaries, branch-comparison advice, and commit classification. every ai
  feature degrades to a deterministic fallback when no api key is present, so
  the tool is fully functional without ai.
- `internalcmd.go` is the dispatcher that the python bridge calls. it reads a
  json request on stdin, runs one operation, and writes a json response on
  stdout. this is the seam between python and go.
- `cmd/vit/main.go` is the command line. it parses arguments in a way compatible
  with the previous python argparse interface and implements every user-facing
  command and its printed output.

note: in this distribution, `validator.go`, `mergeutils.go`, and `aimerge.go`
ship as interface-preserving stubs. see the distribution note at the end.

---

## 8. the python bridge

the python package in `vit/` exists only for the resolve plugin. it has two
kinds of files.

the first kind genuinely needs the resolve api and stays as real python:
`serializer.py` reads a live resolve timeline into the domain json, and
`deserializer.py` rebuilds a resolve timeline from that json. alongside them,
`models.py` and `json_writer.py` define the data classes and the json writer on
the python side.

the second kind used to contain logic and is now a thin shim: `core.py`,
`differ.py`, `validator.py`, `merge_utils.py`, and `ai_merge.py`. each shim
exposes the same functions with the same signatures as before, but the body
simply forwards to the go binary. the shim locates the binary (via the
`VIT_BINARY` environment variable, the path, or `~/.vit/bin`), runs
`vit internal <op>` with a json payload on stdin, and parses the json response.

the protocol is deliberately small. a request is a json object with the fields
one operation needs. a response is `{"ok": true, "result": ...}` on success or
`{"ok": false, "error": "..."}` on failure, and the shim turns a failure into
the same `GitError` the plugin already expected. because the signatures and
error behavior match the originals exactly, the five resolve plugin scripts did
not change at all.

a small amount of logic stays in python on purpose: `find_project_root` and the
commit categorization heuristic are pure functions with no git dependency, so
they run locally without paying for a subprocess.

---

## 9. the resolve plugin

the plugin is the primary interface for real users. it is launched from
resolve's scripts menu and presents commit, branch, merge, switch, push, pull,
and status as panel actions.

the panel has a launcher that runs inside resolve's python and a ui that runs as
a separate process, communicating over a local socket. the launcher handles
everything that needs the resolve api (reading and rebuilding the timeline) and
forwards the rest to the go binary through the shims. the ui is available as a
pyside6 panel with a tkinter fallback.

restoring a timeline is more delicate than saving one, because resolve's api has
gaps: some properties are write-only, some are static-only, and there is no api
to delete a clip or a timeline. the deserializer works within those limits, and
the merge flow has a special path that restores only the overlays produced by an
overlay-aware merge rather than rebuilding the whole timeline.

---

## 10. the merge model in depth

merging happens in layers, from cheapest to most involved.

first, git itself. because the domains are split into separate files, most
merges are handled entirely by git with no conflict. an editor's `cuts.json`
change and a colorist's `color.json` change merge automatically.

second, post-merge validation. after any merge, vit re-reads the project and
runs the validation engine. this catches problems git cannot see, because git
merges files independently and does not understand that a clip id in one file
refers to a clip in another. the classic case: one branch deletes a clip while
another grades it. git merges both files cleanly, but the result has a grade
pointing at a clip that no longer exists. validation flags it.

third, overlay-aware resolution. when two branches collide on the same clip id
in an incompatible way, for example one branch keeps a media clip and the other
replaces it with a title generator, a plain "take one side" merge would lose
work. the overlay policy keeps both: it moves one onto a higher video track as
an overlay, mints a fresh id for it, and rewrites the names of the generator and
grade sidecar files so the two clips do not share assets. it also reconciles the
track counts in the metadata.

fourth, ai-assisted semantic merge. for genuinely ambiguous cases, the ai layer
compares the base, ours, and theirs versions of every domain file and proposes a
per-domain decision (accept ours, accept theirs, merge, or ask the user) with a
confidence level. high-confidence decisions are applied automatically;
low-confidence ones are presented to the user, whose answers are fed back to
produce the final resolution. this layer is optional and only runs on the
command line; the panel never blocks on it.

---

## 11. command flows

a few representative flows, to make the layering concrete.

`vit init`:
- create the git repository and the `.vit/` config.
- write empty domain files (byte-identical to what the python serializer would
  write for an empty timeline).
- write the media-ignoring `.gitignore`.
- make the initial snapshot commit.

`vit commit`:
- stage the timeline and asset files.
- if no message is given and ai is available, suggest one from the diff.
- commit, and report the short hash.

`vit merge <branch>`:
- auto-save any uncommitted changes first.
- optionally run a pre-merge ai analysis and, on a manual-review recommendation,
  ask the user to proceed.
- run the git merge.
- if it succeeds, run validation, detect whether both branches touched the same
  domains, and, if needed, run the ai review.
- if it conflicts, list the conflicted files and, unless ai is disabled, offer
  ai-assisted resolution.

`vit diff`:
- read the old version of each domain file from a git ref and the new version
  from disk, then render the human-readable diff. if there is no history yet,
  fall back to a raw git diff.

---

## 12. testing strategy

the core has two test layers.

go tests cover the git wrapper against a temporary repository, the differ's
exact output, the json byte-parity round trip, and, in the full build, the
validator, the overlay merge, and the ai layer with a stubbed transport.

python tests cover the serializer against a mock resolve api, the deserializer,
the plugin utilities, and the go-shim integration (which requires the built
binary on the path or via `VIT_BINARY`).

the port from python to go was verified with differential testing: the same
inputs were run through the original python and the new go, and the outputs were
compared byte for byte, including the tricky numeric formatting in speed and
color diffs, the ordering of validation findings, and the overlay merge plan.

---

## 13. build and install

the cli is a go binary:

    go build -o ~/.vit/bin/vit ./cmd/vit

put `~/.vit/bin` on the path. for resolve integration, install the python
package with `pip install .` and link the plugin scripts with
`vit install-resolve`. set `GEMINI_API_KEY` in the environment or a project
`.env` file to enable ai features; everything works without it.

---

## 14. distribution note

this public distribution ships the three engines that constitute vit's
proprietary core as interface-preserving stubs:

- `internal/vit/mergeutils.go`, the overlay-aware merge-conflict resolution
  engine.
- `internal/vit/aimerge.go`, the ai semantic merge and enrichment engine.
- `internal/vit/validator.go`, the post-merge validation engine.

everything else, the architecture, the command line, the json layer, the differ,
the git wrapper, the dispatcher, and the entire python bridge and resolve plugin,
is the real implementation. the project builds, installs, and runs end to end.
with the engines stubbed, validation reports no issues, overlay merges pass the
input through unchanged, and ai features return their no-key fallbacks. the full
implementation of the three engines is available under nda.
