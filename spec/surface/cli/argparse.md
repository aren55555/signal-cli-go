# argparse4j 0.9.0: text layout, parsing and error algorithms (as used by signal-cli 0.14.7)

This file describes, precisely enough to reproduce byte for byte, how signal-cli's
bundled argument parser (argparse4j 0.9.0) lays out help and usage text, wraps and
justifies paragraphs, parses arguments, and words its errors and "Did you mean"
suggestions. It refines [index.md](index.md) §1, §4 and §5. Where this file and
index.md disagree, this file wins (see §12 for the list of corrections).

Machine-checkable vectors: [`testdata/vectors/argparse_text.json`](../../../testdata/vectors/argparse_text.json)
(described in §11).

## Sources

| What | Version / pin | Location |
|---|---|---|
| argparse4j source (MIT) | tag `argparse4j-0.9.0` (tag object `f66cb9a6700ac8b6d7e0ea75290424a196b24e2d`, commit `539dce0292366182b4e287ccc644230c7fd99963`, 2021-04-09) | https://github.com/argparse4j/argparse4j/tree/argparse4j-0.9.0 — read: `main/src/main/java/net/sourceforge/argparse4j/{ArgumentParsers,ArgumentParserBuilder,DefaultSettings}.java`, `internal/{ArgumentParserImpl,ArgumentImpl,ArgumentGroupImpl,SubparsersImpl,SubparserImpl,ParseState,TerminalWidth,ArgumentParserConfigurationImpl,Unrecognized*Exception}.java`, `helper/{TextHelper,PrefixPattern,ASCIITextWidthCounter,MessageLocalization,TypeNameLocalization}.java`, `impl/type/{ReflectArgumentType,EnumStringArgumentType,StringArgumentType}.java`, `impl/choice/CollectionArgumentChoice.java`, `impl/action/{Help,Version,Count,StoreTrue,Store}ArgumentAction.java`, `inf/ArgumentParserException.java`, `main/src/main/resources/.../internal/ArgumentParserImpl{,_en}.properties` |
| argparse4j jar as shipped | `lib/argparse4j-0.9.0.jar`, sha256 `9eb3b54043038bc111bf0e15a8f69895244f92e3c2bc2800158578ca711f6117` (Maven `pom.properties` version 0.9.0, built 2021-04-09; cost constants checked with `javap`) | `/work/third_party/signal-cli-0.14.7/lib/` |
| signal-cli source (parser configuration only) | tag `v0.14.7`, commit `b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8` | `src/main/java/org/asamk/signal/{App,Main}.java`; `src/main/java/org/asamk/signal/commands/*` (grep of `.type(`, `.nargs(`, `.choices(`, `action(`, positional names only) |
| JDK line BreakIterator | OpenJDK 25.0.5+8 (Temurin), the JVM the reference binary runs on | black-box only: the segmentation table in §2 was learned by querying `java.text.BreakIterator.getLineInstance()`; no JDK source was read |
| Reference binary | signal-cli 0.14.7 release, Java 25 | `/work/third_party/signal-cli-0.14.7/bin/signal-cli`, run with `--config <scratch dir>`, stdin `/dev/null`, `LANG=C.UTF-8` unless stated |
| Existing specs | — | [index.md](index.md) §1, §4, §5; [observations.md](observations.md); `testdata/golden/cli/recorded/*.json` |

All behavior described here was checked against the running jar (the vectors file
was produced by executing it), and the signal-cli-specific parts against the binary.

## 0. Conventions

- **Width** of a string means its length in UTF-16 code units (each BMP character
  counts 1, a non-BMP character counts 2; tabs, newlines and other controls count 1).
  This is the width counter used for every locale except Japanese, Chinese and Korean
  (§0.2).
- **Line separator** is `\n` (the JVM's separator on Linux). Every "newline" below is
  `\n`.
- **W** is the format width (§1). **Prog** is the program name, `signal-cli` for
  every parser, including subcommand parsers.
- Option strings are called **flags** (`-a`, `--account`); a **positional** is an
  argument declared with a bare name (`recipient`). The **textual name** of an
  argument is its flags joined with `/` (`-a/--account/-u/--username`) or, for a
  positional, its name.
- The **dest** of a flag argument is: the explicit dest if one was set; otherwise
  the first flag that starts with two or more dashes (or, if none does, the first
  flag), with leading dashes removed and every `-` replaced by `_`
  (`-d --data-dir -c --config` gives `data_dir`). A positional's dest is its name
  with `-` replaced by `_` (`verification-code` gives `verification_code`).

### 0.1 Prefix characters

The only prefix character is `-`. A token is **flag-shaped** if it starts with one or
more `-` and is not made of dashes only. Removing the leading run of dashes from a
flag-shaped token gives its **body** (`---verbose` has body `verbose`). A
flag-shaped token is **short-shaped** if its leading dash run is exactly one dash.

### 0.2 Locale

Message texts, section headers and the `usage:` / `default:` words come from a
resource bundle chosen by the JVM default locale, which is derived from the POSIX
locale environment and only takes effect when that locale is installed on the
system. The bundle has German, Dutch and Russian translations; every other language
uses the English texts given in this file. If the default language is `ja`, `zh` or
`ko`, a CJK-aware width counter replaces the one in §0. The recorded transcripts and
all vectors use English with the plain counter: in this sandbox only C/POSIX locales
exist, so `LANG=de_DE.UTF-8` still produced English (verified). A reimplementation
should always use English and the plain counter (see Gaps).

## 1. Format width

The parser computes its width W once, when the parser is built:

1. If the environment variable `COLUMNS` is set: parse it as a decimal integer (an
   optional leading `+` or `-` is accepted, nothing else, no surrounding spaces). If
   parsing fails the terminal width is "unknown".
2. If `COLUMNS` is not set: ask the controlling terminal. The reference runs
   `sh -c "stty -a < /dev/tty"` and takes N from `columns N`. This consults
   `/dev/tty`, not stdout, so redirecting output does not change it. If there is no
   controlling terminal (or the command fails), the width is unknown.
3. W = terminal width − 5. If that is ≤ 0 (including "unknown", which counts as −1),
   W = **75**.

Examples: `COLUMNS` unset and no tty gives 75 (every recorded transcript);
`COLUMNS=80` gives 75; `COLUMNS=120` gives 115; `COLUMNS=45` gives 40; `COLUMNS=5`
gives 75; `COLUMNS=6` gives 1; `COLUMNS=abc` gives 75. Note that an interactive shell
usually does **not** export `COLUMNS`, so in a terminal the width is the terminal
width minus 5.

The same W is used for usage lines, help rows, description, epilog and the error
line, in every parser (subcommand parsers share the top-level W).

## 2. Line-break opportunities

Paragraph wrapping (§3) cuts text into **segments**. A segment is the text between
two consecutive break opportunities. Segments carry their trailing whitespace: in
`Raise log level` the segments are `Raise `, `log `, `level`. The reference uses the
JDK's line BreakIterator for this. The table below reproduces it exactly for every
ASCII input. It was learned black-box and then checked with zero mismatches against
all 883,708,280 class strings of length ≤ 8, 2,000,000 random ASCII strings of length
≤ 60, and every segmentation vector.

### 2.1 Character classes

| Class | Characters |
|---|---|
| W | every ASCII character not listed below: letters, and `# & * + / < = > @ \ ^ _ ` `` ` `` ` | ~` |
| G | `0`–`9` |
| N | `,` `.` |
| D | `-` |
| P | `!` `%` `:` `;` `?` `]` `}` |
| E | `)` |
| L | `(` |
| O | `[` `{` |
| C | `$` `'` |
| Q | `"` |
| S | space, U+007F, and every C0 control not in B or R (U+0000–U+0002, U+0004–U+0008, U+000B, U+000E–U+001F) |
| B | TAB U+0009, LF U+000A, FF U+000C, U+0003 |
| R | CR U+000D |

### 2.2 Segmentation automaton

Start in state 0 at the current position. Feed characters one by one, following the
table. An `x` means "no transition": stop before that character. Also stop at the end
of the text. The next break is right after the last character that left the
automaton in an **accepting** state. Every transition out of state 0 lands in an
accepting state, so each segment holds at least one character. Then start again in
state 0 at the break. The end of the text is always a break, and there is never a
break at position 0.

| state | acc | W | G | N | D | P | E | L | O | C | Q | S | B | R |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 0 | no | 1 | 2 | 3 | 4 | 3 | 3 | 5 | 5 | 16 | 5 | 6 | 7 | 8 |
| 1 | yes | 1 | 1 | 3 | 9 | 3 | 3 | 10 | x | 17 | 3 | 6 | 7 | 8 |
| 2 | yes | 1 | 2 | 11 | 9 | 3 | 3 | 10 | x | 17 | 3 | 6 | 7 | 8 |
| 3 | yes | x | x | 3 | x | 3 | 3 | 10 | x | 17 | 3 | 6 | 7 | 8 |
| 4 | yes | x | 12 | x | 4 | x | x | 13 | 14 | 24 | 14 | 6 | 7 | 8 |
| 5 | yes | 1 | 2 | 3 | 4 | 3 | 3 | 5 | 5 | 16 | 5 | 6 | 7 | 8 |
| 6 | yes | x | x | x | x | x | x | 10 | x | x | x | 6 | 7 | 8 |
| 7 | yes | x | x | x | x | x | x | x | x | x | x | x | x | x |
| 8 | yes | x | x | x | x | x | x | x | x | x | x | x | 7 | x |
| 9 | yes | x | x | x | 9 | x | x | 10 | x | 17 | x | 6 | 7 | 8 |
| 10 | no | x | x | x | x | x | 15 | x | x | x | x | x | x | x |
| 11 | yes | x | 12 | 3 | x | 3 | 3 | 10 | x | 17 | 3 | 6 | 7 | 8 |
| 12 | yes | x | 12 | 11 | 9 | 3 | 3 | 10 | x | 17 | 3 | 6 | 7 | 8 |
| 13 | no | x | 12 | x | 14 | x | 15 | 14 | 14 | 14 | 14 | x | x | x |
| 14 | no | x | 12 | x | 14 | x | x | 14 | 14 | 14 | 14 | x | x | x |
| 15 | yes | x | x | 15 | x | 15 | 15 | x | x | x | 15 | x | 7 | 8 |
| 16 | yes | 1 | 2 | 3 | 18 | 3 | 3 | 16 | 16 | 28 | 16 | 6 | 7 | 8 |
| 17 | yes | 1 | 2 | 3 | 25 | 3 | 3 | 17 | 17 | 17 | 17 | 6 | 7 | 8 |
| 18 | yes | x | 12 | x | 19 | x | x | 13 | 14 | 20 | 14 | 6 | 7 | 8 |
| 19 | yes | x | 12 | 11 | 19 | 3 | 3 | 13 | 14 | 20 | 21 | 6 | 7 | 8 |
| 20 | yes | 1 | 2 | 11 | 22 | 3 | 3 | 26 | 26 | 26 | 26 | 6 | 7 | 8 |
| 21 | yes | x | 12 | 3 | 14 | 3 | 3 | 13 | 14 | 26 | 21 | 6 | 7 | 8 |
| 22 | yes | x | 12 | x | 23 | x | x | 13 | 14 | 26 | 14 | 6 | 7 | 8 |
| 23 | yes | x | 12 | x | 23 | x | x | 13 | 14 | 14 | 14 | 6 | 7 | 8 |
| 24 | yes | 1 | 2 | 3 | 27 | 3 | 3 | 24 | 24 | 26 | 24 | 6 | 7 | 8 |
| 25 | yes | x | 12 | x | 25 | x | x | 13 | 14 | 17 | 14 | 6 | 7 | 8 |
| 26 | yes | 1 | 2 | 3 | 22 | 3 | 3 | 26 | 26 | 26 | 26 | 6 | 7 | 8 |
| 27 | yes | x | 12 | x | 27 | x | x | 13 | 14 | 26 | 14 | 6 | 7 | 8 |
| 28 | yes | 1 | 2 | 3 | 29 | 3 | 3 | 28 | 28 | 28 | 28 | 6 | 7 | 8 |
| 29 | yes | x | 12 | x | 30 | x | x | 13 | 14 | 20 | 14 | 6 | 7 | 8 |
| 30 | yes | x | 12 | 11 | 30 | 3 | 3 | 13 | 14 | 20 | 31 | 6 | 7 | 8 |
| 31 | yes | x | 12 | 3 | 14 | 3 | 3 | 13 | 14 | 14 | 31 | 6 | 7 | 8 |

The same table is in the vectors file (`line_break.dfa`). States 16–31 only matter
for unusual runs of `$`, `'`, `"` and `-`; states 0–15 cover ordinary prose.

### 2.3 What this means in practice (informative)

- Whitespace sticks to the preceding text; the break comes after a run of spaces.
  Spaces at the start of the text form a segment of their own (`  ` + `a`).
- `/`, `_`, `=`, `@`, `#`, `&`, `+`, `<`, `>`, `|`, `~` are word characters: there is
  **no** break at a slash (`$XDG_DATA_HOME/signal-` is one piece) or inside `a=b`.
- A dash run allows a break after it when a word character, `(`, `[`, `{`, `"`,
  `!`, `.` and similar follow (`on-` `first-` `use`; `line-` `delimited`;
  `-` `a/--` `account/-` `u/--` `username: `). At the start of a segment a dash run
  followed by a digit forms a signed number with no break (`-5`, `--5`, `-1.5`),
  but after a word or number a dash breaks before a digit (`a-` `5`, `1-` `2`).
- Closing punctuation (`. , : ; ! ? % ) ] }`) sticks to the preceding text and allows
  a break after it (`signal.` `art`, `e.` `g.`), except inside numbers: `1.5`,
  `1,000.25` and `-1.5.6` are unbroken, while `a.5`, `1.a` and `.5` break after the
  dot.
- `(`, `[`, `{`, `$`, `'`, `"` stick to what follows. A break is allowed **before**
  a `(`, `[` or `{` that directly follows a word character or a number (`a` + `(b)`,
  `a` + `[b]`). The exception is an empty `()` directly after something: `a()`,
  `a.()` and `0()` stay whole. `$` and `'` inside a word never break (`it's`, `a$b`,
  `a$(`).
- `"` is an opener at the start of a segment and a closer after a word (`"a"` `b`).
- TAB, LF, FF, U+0003 end a segment unconditionally (mandatory break after them); CR
  ends one unless LF follows (`\r\n` together).

### 2.4 Non-ASCII characters

Measured equivalences, each checked over 30,000 random contexts: U+2019 `’`,
U+201D `”`, U+203A, U+00BB, U+00A2, U+00B0, U+2030, U+2032, U+3001, U+3002,
U+FF01, U+FF09, U+FF0C behave as **P**. U+2018 `‘`, U+201C `“`, U+201E, U+2039,
U+00AB, U+20AC, U+00A3, U+00A5, U+FF08 behave as **O**. U+2010, U+2012–U+2015 and
U+00AD behave as **D**. U+2002, U+2003, U+2009, U+3000 and U+0085 behave as **S**.
U+2028 and U+2029 behave as **B**. U+0660, U+00B2 and U+00BD behave as **G**. Latin,
Cyrillic, Hebrew and Arabic letters, U+00BF, U+00A1, U+00A7, U+00B6, U+2022, U+2026,
U+00B7, U+00D7, U+00F7, U+00A9 and U+2122 behave as **W**. In general: opening and
initial-quote punctuation and currency signs (except `¢`) act like O; closing and
final-quote punctuation like P; dash punctuation like D; space separators like S;
digits like G; other letters and symbols like W.

Not modelled (they have no ASCII equivalent): no-break spaces (U+00A0, U+2007,
U+202F, U+2011, U+FEFF), zero-width and combining characters (U+200B, U+200C,
U+200D, U+0301 and other Mn/Me/Cf), CJK ideographs, kana and hangul (each is its own
segment), Thai (dictionary-based breaking), and Devanagari danda. The only non-ASCII
character in signal-cli's own help texts is U+2019 (`Don’t ...`, class P, so
`Don’` + `t`).

## 3. Paragraph wrapping and justification

All wrapped text (description, epilog, help descriptions, the error line, subparser
descriptions) goes through one procedure. Its inputs are the text, W, a starting
column `offset` (how many columns are already used on the first line before the
text starts), a first-line indent string and a continuation indent string.

| Use | offset | first indent | continuation indent |
|---|---|---|---|
| parser description, epilog | 0 | `""` | `""` |
| error line `signal-cli: error: <msg>\n` | 0 | `""` | `""` |
| help-row description (§4) | 25 | `""` | 25 spaces |
| subparsers description, group description | 2 | `""` | 2 spaces |

### 3.1 Filling

Keep an output buffer (starting with the first indent), a current-line buffer
(empty), and a running column count `col` = offset + width(first indent). For each
segment in order (§2):

1. Add the segment's width to `col`. The width includes the segment's trailing spaces
   and, if present, its trailing newline character.
2. If `col` > W: justify the current-line buffer (§3.2) using the column count
   *before* this segment, append the result, a newline and the continuation indent to
   the output, empty the current-line buffer, and set `col` = width(continuation
   indent) + width(segment).
3. Append the segment to the current-line buffer.
4. If the segment ends with a newline: append the current-line buffer **unchanged**
   (not justified, trailing spaces kept, newline included) plus the continuation
   indent to the output. Empty the buffer and set `col` = width(continuation indent).

At the end, append the current-line buffer unchanged. The result is not followed by
an extra newline; callers add one where noted.

Consequences, all visible in the reference:

- **Last line**: the last line of the text, and every line ended by an embedded
  newline, is never justified and keeps its own spacing, including trailing spaces.
- **The overflow test counts trailing spaces and the trailing newline.** A word that
  would end exactly at column W is still pushed to the next line if a space follows
  it. A final `word\n` that ends exactly at W is pushed down too, because the newline
  counts as one column. Example: a 74-character text plus `\n` wrapped at W=74 puts
  its last word on a second line.
- **Over-long segments** are never split. A segment wider than the remaining space
  starts a new line and sticks out past W. If such a segment is the first thing on a
  line (the current-line buffer is empty), step 2 still runs: it emits an **empty**
  justified line, so the output gets a line holding only the indent. With offset 0
  and a first segment wider than W the text starts with `\n`. In a help row whose
  first segment is wider than W − 25, there is a line of exactly 25 spaces before
  the segment (§4).
- **Embedded newlines** start a new line that begins with the continuation indent.
  Text after the newline (including leading spaces) is kept as is. Two newlines in a
  row give an empty line holding just the continuation indent.
- A TAB, FF or CR is a break opportunity but not a line end: it stays in the line
  and counts one column.

### 3.2 Justifying a full line

Inputs: the line text, W, and `cur` = the column count after the line's last segment
(it includes the line's start column, i.e. offset/indent, and its trailing spaces).

1. Strip leading and trailing ASCII spaces (U+0020 only) from the line, reducing
   `cur` by the number removed.
2. Count the **gaps**: maximal runs of spaces inside the stripped line. If there are
   none, the line is used as is (it may be shorter than W, or longer if it is one
   over-long segment).
3. needed = W − cur. Each gap gets `needed div gaps` extra spaces. The remainder
   r = `needed mod gaps` is handed out one each to a block of r consecutive gaps
   starting at gap index s = (gaps − r + 1) div 2 (gaps counted from 0, left to
   right): gaps s, s+1, …, s+r−1 get one more space. So the extra space is centred,
   rounded towards the right. `div` and `mod` truncate toward zero; if `needed` is
   negative nothing is inserted. Each gap keeps its original spaces plus the extra.

Worked example 1, the error line at W=75. The segments of
`signal-cli: error: argument -a/--account/-u/--username: expected one argument\n`
are `signal-` `cli: ` `error: ` `argument ` `-` `a/--` `account/-` `u/--`
`username: ` `expected ` `one ` `argument\n`. The running column after `one ` is
69, and adding `argument\n` (9) gives 78 > 75. So the line
`signal-cli: error: argument -a/--account/-u/--username: expected one ` (cur 69)
is justified: stripping the trailing space gives cur 68. There are 5 gaps (after
`signal-cli:`, `error:`, `argument`, `username:`, `expected`); needed 7: each gap
gets 1, r=2, s=(5−2+1) div 2=2, so gaps 2 and 3 get one more. Result:
`signal-cli:  error:  argument   -a/--account/-u/--username:   expected  one`
then `argument` on the next line (ending with the newline of the format).

Worked example 2, a help row at W=75 (offset 25). `Raise log level and include lib
signal logs. Specify ...`: after `logs. ` the column is 70; `Specify ` would make it
78. The line `Raise log level and include lib signal logs. ` (cur 70, stripped 69)
has 7 gaps, needed 6: each gets 0, r=6, s=(7−6+1) div 2=1, so gaps 1–6 get one space:
`Raise log  level  and  include  lib  signal  logs.`

## 4. Help rows

Every argument row and subcommand row is printed as follows, with a title (the left
column) and a help text (already including any `(default: …)` suffix, §6.3):

- Print two spaces and the title.
- If the help text is empty: newline, done (no trailing spaces).
- If width(title) ≤ 21: pad with spaces so the description starts at column 25
  (0-based), i.e. 25 − 2 − width(title) spaces (at least 2).
- If width(title) > 21: newline, then 25 spaces.
- Then the help text wrapped (§3) with W, offset 25, first indent `""`, continuation
  indent 25 spaces, followed by a newline.

So descriptions start in the 26th column, and continuation lines are indented 25
spaces. Subcommand rows pass a title that already starts with two spaces (§6.4),
so they appear indented by 4 and the 21-limit applies to the name plus 2.

## 5. Usage line

### 5.1 Prefix and indentation

The usage starts with `usage: ` + prog, where for a subcommand parser prog is
followed by the chain of commands (§5.2), all as separate items. Let U =
width(`usage: ` + prog), which is 17 for `usage: signal-cli`.

- If U ≤ 30: the items continue on the same line; continuation lines are indented
  by U spaces, and each item is preceded by one space. Every continuation line
  therefore starts with U+1 = 18 spaces for signal-cli, for the top-level and every
  subcommand.
- If U > 30: `usage: ` + prog is printed alone, followed by a newline, and all items
  go on following lines indented by 6 spaces (plus the one separating space,
  so they start in column 7).

### 5.2 Items, in order

1. For a subcommand parser, first the items of every enclosing parser, outermost
   first: that parser's command name (if it is itself a subcommand), its required
   flag arguments that are not in a mutually exclusive group, its required mutually
   exclusive groups (rendered as in item 4, parenthesised), a single-member group's
   member if the group or the member is required, and its positionals. For
   signal-cli this contributes nothing (the top-level parser has no required
   options, required groups or positionals).
2. The parser's own command name, for a subcommand parser (`send`).
3. Every flag argument not hidden and not in a mutually exclusive group, in
   declaration order, rendered by its short syntax (§5.3).
4. Every mutually exclusive group, in creation order. If it has two or more visible
   members it becomes **separate items**: `[` + first member's syntax without
   brackets, then for every further member an item `|` and an item with that
   member's syntax without brackets, with `]` appended to the last one. A required
   group uses `(` and `)` instead. Example items: `[--dbus`, `|`, `--dbus-system]`.
   A one-member group renders that member normally, or without brackets if the group
   is required.
5. Every visible positional, in declaration order, rendered by its metavar form
   (§5.3).
6. If the parser has subcommands: the choice list `{a,b,…}` (all command names in
   declaration order, comma-separated, no spaces; the subparsers' metavar if one was
   set) followed by the item `...`.

### 5.3 Short syntax and metavars

The **metavar list** of an argument is: the explicit metavar(s) if set; else, if the
argument has choices, the single entry `{c1,c2,…}` (choices in their given order,
comma-separated); else, if its type supplies one, that (`{true,false}` for boolean,
`{v1,v2,…}` with the display strings for display-string enums such as
`{plain-text,json}`, `{NAME1,NAME2}` with the constant names for name-matched enums);
else the dest in upper case for flags (`DATA_DIR`, `GLOBAL-BUS-NAME`) or the dest
unchanged for positionals (`recipient`, `verification_code`). Call the first entry M
and the second entry (or M again if there is only one) M2.

The **value form** depends on nargs (empty for arguments that take no value, such as
`store_true`, `count` and help):

| nargs | value form |
|---|---|
| (unset: exactly one value) | `M` |
| `?` | `[M]` |
| `*` | `[M [M2 ...]]` |
| `+` | `M [M2 ...]` |
| n (integer) | n entries separated by single spaces: entry i is metavar i, and once the list runs out the last metavar repeats (`A B`, `X X X`) |

A flag argument's short syntax is its **first** flag, then (if the value form is
non-empty) a space and the value form, all inside `[` `]` unless the argument is
required: `[-h]`, `[-o {plain-text,json}]`, `[-g [GROUP_ID [GROUP_ID ...]]]`,
`--req REQ`. A positional's syntax is just its value form: `pin`,
`[recipient [recipient ...]]`, and for nargs 1 `recipient`. Marking a positional
"required" changes nothing.

### 5.4 Line filling

Keep a column count, which starts at U when U ≤ 30. When U > 30 it starts at **12**:
the reference counts the 6-column indent twice on the first item line, so that line
breaks earlier than its real length would require. This is a quirk to reproduce.
For each item, in order:

- If it is not the first item and count + 1 + width(item) > W: newline, then the
  continuation indent (U spaces, or 6 spaces when U > 30), then a space and the item;
  count = width(indent) + 1 + width(item).
- Otherwise: a space and the item; count += 1 + width(item).

The first item always goes on the first line. The usage ends with a newline. Items
are never split, so long items (the 61-name choice list) stick out past W, and the
choice list and `...` usually each get their own line. Example (W=75): see the
top-level usage in observations.md; at W=40, `[--dbus |` and `--dbus-system]` land
on different lines.

## 6. Help screen

### 6.1 Layout

In this order:

1. The usage (§5).
2. If the description is non-empty: an empty line, the description wrapped (§3,
   offset 0), a newline.
3. If there is at least one positional (not in a separately-shown group), or the
   parser has subcommands and the subparsers have neither title nor description: an
   empty line, `positional arguments:`, one row per positional (§4), then (in the
   untitled case only) the subcommand rows (§6.4).
4. If there is at least one flag argument (not in a separately-shown group): an
   empty line, `named arguments:`, one row per flag argument in declaration order.
   Members of untitled mutually exclusive groups appear here, at their declaration
   position (`--dbus`, `--dbus-system` right after `-a`). Hidden arguments have no
   row. `-h, --help` (`show this help message and exit`) is the first flag argument
   of every parser.
5. If there are subcommands and the subparsers have a title or a description: an
   empty line, the title (or `subcommands` if only a description was given) followed
   by `:`. If a description is set, the next line is two spaces plus the description
   wrapped with offset 2 and indent 2 spaces, then an empty line. Then the
   subcommand rows. signal-cli sets the title `subcommands` and no description.
6. Separately shown argument groups (titled or described groups, which signal-cli
   does not use): each preceded by an empty line, with the title and `:`, the
   description, a line about mutual exclusivity for mutex groups, and the rows.
7. If the epilog is non-empty: an empty line, the epilog wrapped (§3, offset 0), a
   newline.

### 6.2 Row titles

A positional's title is its first metavar entry (§5.3), e.g. `recipient`. A flag
argument's title lists **every** flag, each followed by a space and the value form
when that form is non-empty, joined by `, `: `-h, --help`,
`-d DATA_DIR, --data-dir DATA_DIR, -c DATA_DIR, --config DATA_DIR`,
`-o {plain-text,json}, --output {plain-text,json}`,
`-g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]`.

### 6.3 Default annotations

A parser shows defaults only if it was configured to. In signal-cli that is only the
top-level parser; subcommand parsers never show defaults. When enabled, a row gets
the suffix `(default: X)` if the argument's stored default value is not null: one
space then the suffix when the help text is non-empty, or the suffix alone when it
is empty. X is the Java `toString()` of the default:

| type of default | X |
|---|---|
| boolean | `true` / `false` |
| integer | decimal (`0`, `-1`) |
| string | the string as is (`org.asamk.Signal`) |
| enum | the enum's display string (`live`, `on-first-use`, `json`) |
| double | Java's double formatting: `3.0`, `-1.0`, `1.0E10`, `NaN` (not needed for signal-cli, whose only double default is in a subcommand) |
| list | `[a, b]` |
| file | the path |

`count` and `store_true` arguments carry defaults of 0 and false even when not set
explicitly (signal-cli sets them explicitly too). `-h` and `--version` have no
default, so they show no suffix. A `*`/`+` positional with no explicit default has a
null stored default and no suffix. At the top level of signal-cli this gives
`(default: 0)` for `-v`, `(default: false)` for `--scrub-log`, `--dbus`,
`--dbus-system`, `--disable-send-log`, `(default: org.asamk.Signal)` for
`--bus-name`, `(default: live)` and `(default: on-first-use)` for the enums, and
configured values from the config file (index.md §7). `-o` and `-d` have no default
unless one is configured.

### 6.4 Subcommand rows

First comes one row for the choice list: title `{a,b,…}` (same text as in §5.2) and
the subparsers' help text. signal-cli sets none, so this is just two spaces, the
list and a newline, never wrapped. Then, for each command in declaration order that
has a **non-empty** help text, a row with title = two spaces + command name (aliases,
which signal-cli does not use, would be appended in parentheses), so names appear in
column 4 and descriptions in column 26. A command name longer than 19 characters
(e.g. `deleteLocalAccountData`, `sendMessageRequestResponse`,
`sendPaymentNotification`) puts its description on the next line. A command without
help text (`version`) gets **no row**, though it appears in the choice list.

### 6.5 The help action

`-h`/`--help` takes no value. When the parser meets it (§7.2), the help screen of
**the parser that owns the flag** is printed to stdout immediately and parsing stops
with "help shown" (exit 0 in signal-cli). Tokens after it are never examined; errors
raised earlier in the token stream win. Deferred checks (§7.5) are never reached.

## 7. Parsing

### 7.1 Token kinds

A token is treated as a flag (option) if it is flag-shaped (§0.1), unless:

- the parser has passed a `--` separator (§7.2), or
- the token is a negative integer, `-` followed only by ASCII digits (`-5`, `-12`),
  and the parser does not itself define a flag of that shape. If it does (e.g. a
  `-1` flag), every negative-integer-looking token is a flag. `-3.5` and `-5x` are
  always flags.

`--` on its own is special (§7.2). `-` on its own is not flag-shaped, so it is a
positional or a value. Note that `-5` is not a flag but `--5` is.

### 7.2 Main loop

The tokens after the program name are processed left to right by the top-level
parser. When a subcommand is selected the remaining tokens go to that subcommand's
parser, which runs the same loop.

**A flag token.** Split it at the first `=`: flag part before it, attached value
after it (no `=`: no attached value). Resolve the flag part:

1. If it equals one of the parser's flags exactly, that argument is selected.
2. Otherwise collect all of this parser's flags (short and long) that **start with**
   the flag part. If there are none, go to "unknown flag" below.
3. If there are candidates and the flag part is short-shaped and could also be read
   as concatenated short flags (§ below; checked on the flag part), the short flag
   formed by its first two characters joins the candidate list and the result is
   ambiguous. signal-cli has no multi-character single-dash flags, so this never
   happens there.
4. Exactly one candidate: selected (`--vers` is `--version`, `--dbus-` is
   `--dbus-system`, `--di` is `--disable-send-log`).
5. Several candidates: error `ambiguous option: <flag part> could match <c1>, <c2>, …`,
   candidates sorted by plain string order and joined by `, `.

An argument selected this way is first checked against mutual exclusion (§7.5),
then processed with the attached value if any (§7.3). Example: `-d=x` and
`--data=x` give the value `x`.

**Unknown flag.** If resolution found nothing:

- If the token (the whole token, including any `=`… part) is short-shaped, it is
  read as concatenated short flags: for each character after the dash, form `-c`.
  If `-c` is not a flag of this parser, stop: the token is unrecognized. If `-c`
  takes no value, run it now (after the mutual-exclusion check) and continue with
  the next character. If `-c` takes a value, it is the selected argument, and the
  rest of the token after that character (if any, `=` included) is its attached
  value: `-ojson` gives `-o json`, `-vvv` counts three times, `-vbv` counts `-v`
  twice and sets `-b`, `-ojs=on` gives `-o` the value `js=on`. If it takes a value
  and nothing follows, the value comes from the next token. If every character was a
  valueless flag, the token is fully consumed.
- Otherwise, or if the reading above stopped at an unknown character, it is an error:
  `unrecognized arguments: '<whole token>'`. It is reported with the **unknown-option**
  kind, which can produce suggestions (§9). The valueless flags already run from the
  token have no visible effect because parsing stops.

**`--`.** The first `--` is consumed and from then on no token is a flag: all
remaining tokens, including later `--` tokens, are positionals.

**A positional token** (not a flag, or after `--`):

- If this parser still has an unfilled positional, the token is collected for it.
  A positional with exactly one value or nargs `?` takes one token and is then
  full. A `*`, `+` or nargs-n positional keeps taking tokens until it reaches its
  maximum (n, or unlimited for `*`/`+`) or a flag token intervenes; after the flag is
  processed, collection resumes for the same positional. So `send +1 -m hi +2` gives
  recipients `+1 +2`.
- Otherwise, if no `--` has been seen and the parser has subcommands, this token
  selects the subcommand (§7.6). First this parser finishes: its positionals are
  distributed (§7.4) and its required checks run (§7.5, deferred). Then the selected
  subcommand parser handles all remaining tokens, and this parser's loop ends.
- Otherwise: error `unrecognized arguments: '<rest>'`, where `<rest>` is **this token
  and every token after it**, joined by single spaces, inside a single pair of
  quotes. Example: `version a b` gives `'a b'`, `-- version -h` gives
  `'version -h'`, and `setPin 1 2` gives `'2'`. This kind never has suggestions.

**End of tokens.** If the parser has subcommands and none was selected: error
`too few arguments` (immediately, before any other end check). Otherwise distribute
positionals (§7.4), then run the required checks (§7.5).

### 7.3 Option values

For an argument that takes no value (`store_true`, `count`, help, version): with an
attached value, error `ignore implicit argument '<value>'` (as an argument error,
§8.2: `argument --scrub-log: ignore implicit argument 'x'`). Otherwise it runs
(store_true sets true, count adds 1, help prints help and stops, version prints the
version line `signal-cli 0.14.7` to stdout and exits 0 at once).

For an argument with exactly one value, or nargs `?`:

- The value is the attached value if there is one. Otherwise it is the next token,
  if one exists and is **not** a flag token (§7.1, `--` counts as a flag here). So
  `-a -5` takes `-5` and `-s -` takes `-`, but `-a --` and `-a -b` take nothing.
- No value: for exactly-one, error `expected one argument`. For `?`, the argument's
  constant (null by default) is stored.
- The value is converted (§10) and stored. A later occurrence of the same option
  overwrites the earlier one (last wins).

For nargs `*`, `+` or n:

- With an attached value: exactly that one value, nothing more is taken (`-g=1 2`
  gives `-g` the list `[1]`; `2` is a positional).
- Otherwise take following tokens until the maximum is reached, the tokens run out,
  or a flag token is met.
- Fewer than the minimum (1 for `+`, n for nargs n): error
  `expected <min> argument(s)` (`expected 1 argument(s)`, `expected 2 argument(s)`).
- Each value is converted. The list is stored even if empty (`-g` alone stores an
  empty list), and a later occurrence replaces the earlier list.

Values are converted as soon as they are taken, so a bad value raises its error
immediately, before later tokens (including `-h`) are looked at.

### 7.4 Distributing positionals

At the end of a parser (or before switching to a subcommand), the collected
positional tokens (a flat list, in order) are dealt to the positionals in
declaration order. Each positional must leave enough tokens for the minimums of the
ones after it (1 for exactly-one, 0 for `?` and `*`, 1 for `+`, n for nargs n). If
the total minimum exceeds the number of tokens: error `too few arguments`. Otherwise
an exactly-one positional takes one token, `?` takes one only if more than the
remaining minimum is left, and `*`/`+`/n take as many as possible (up to their
maximum) while leaving the remaining minimum. A `*`/`+` positional that gets no
tokens keeps its default (an empty list for `*` without an explicit default).
Positional values are converted here, at the end, so their conversion errors come
after all flag processing.

### 7.5 Mutual exclusion, required options, and which error wins

- **Mutual exclusion** is checked each time a flag argument is selected, before its
  value is read: if another member of the same group was already used, error
  `not allowed with argument <earlier member's textual name>`, reported against the
  current argument: `argument --dbus-system: not allowed with argument --dbus`. The
  same member repeated is fine.
- **Required flag arguments**: after positional distribution, every flag argument
  marked required and never used is checked in declaration order. Each missing one
  replaces the previously recorded error, so the error kept names the **last**
  missing one: `argument <textual name> is required`.
- **Required mutually exclusive groups**: checked only if no required-option error
  was recorded. Each group with no used member records
  `one of the arguments <name1> <name2> … is required` (each textual name followed by
  one space; the final space is followed directly by `is`). The last such group
  wins.
- These two checks are **deferred**: they are recorded, not raised. They are raised
  only after the whole token list has been handled (including any subcommand). The
  first deferred error recorded wins: a parent parser's deferred error suppresses the
  subcommand's own required checks. Any immediate error raised later (in the
  subcommand) replaces a deferred one.

Resulting precedence, for errors within one parser: the first immediate error in
token order (unknown option, ambiguous option, bad value, missing value, not-allowed,
ignore-implicit, extra positional) wins. Help (`-h`) wins if it comes before the
first immediate error. Then `too few arguments` (missing subcommand, or too few
positional tokens), then positional conversion errors, then the last missing required
option, then the last missing required group. Example: `getSticker` with neither
option reports `argument --sticker-id is required` (declared after `--pack-id`);
`setPin` with no PIN reports `too few arguments` even though nothing else is wrong.

### 7.6 Choosing a subcommand

The token is matched against the command names: exact match first; otherwise, if
exactly one name **starts with** the token, that command (`vers` runs `version`,
observed on the binary); several: error
`ambiguous command: <token> could match <n1>, <n2>, …` (sorted, `, `-joined; no
suggestions). An empty token, or one that matches nothing, gives error
`invalid choice: '<token>' (choose from '<n1>', '<n2>', …)` with **every** command
name, in declaration order, each in single quotes, separated by `, `. This error has
the **unknown-command** kind and gets suggestions (§9). Errors from choosing the
subcommand are reported by the parser that owns the subcommands (top-level usage).

### 7.7 Known-arguments mode (the logging pre-parser)

signal-cli runs a second, minimal parser first (index.md §8): prog `signal-cli`,
defaults not shown, arguments `-v/--verbose` (count), `--log-file` and
`--scrub-log` (store_true), no help texts. It runs over the whole argument list in a
mode that collects unknown tokens instead of failing:

- An unknown flag token is added to the unknown list (when the short-flag reading
  stopped at character i, the list gets `-` followed by the rest of the token from
  i: `-vx` counts `-v` and lists `-x`; `-v-v` counts once and lists `--v`).
- An extra positional token is added to the unknown list and parsing continues.
- Everything else is as above. In particular abbreviations resolve against **its own**
  three flags (`--ver` and `--verb` mean `--verbose` here, while `--ver` is ambiguous
  for the main parser), `-h` anywhere after earlier successful tokens prints its help
  (the 7-line block, wrapped at W like any help) to stdout, and errors such as
  `--log-file` without a value make signal-cli ignore this pass.

Its help at W=75 is:

```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
```

Then the main parser runs normally. Because both passes print help, a successful `-h`
produces this block immediately followed by the real help. With `-o xml -h` the block
goes to stdout (the pre-parser ignores the unknown `-o xml`) while the main parser
fails on `-o xml` (stderr, exit 1).

## 8. Error output

### 8.1 Format

On a parse error signal-cli writes to **stderr** and exits 1:

1. The usage (§5) of the parser that raised the error. That is the subcommand parser
   for errors inside a subcommand, and the top-level parser for errors while
   choosing the subcommand (§7.6) or in global options.
2. `signal-cli: error: <message>` plus a newline, wrapped and justified as one
   paragraph with W, offset 0, no indents (§3). The newline belongs to the last
   segment and counts toward its width. The prefix is always `signal-cli`, even in
   subcommands.
3. For unknown-option and unknown-command errors only, the suggestion block (§9) if
   any.

Nothing goes to stdout. There is no help and no "try -h" hint.

### 8.2 Messages

"Argument errors" are prefixed `argument <textual name>: `.

| Situation | Message (before wrapping) |
|---|---|
| no subcommand given | `too few arguments` |
| too few positional tokens | `too few arguments` |
| unknown or empty subcommand | `invalid choice: '<tok>' (choose from '<n1>', '<n2>', …)` + suggestions |
| ambiguous subcommand prefix | `ambiguous command: <tok> could match <n1>, <n2>` |
| unknown option | `unrecognized arguments: '<token>'` + suggestions |
| leftover positional tokens | `unrecognized arguments: '<tok> <tok2> …'` (rest of argv, one pair of quotes) |
| ambiguous option prefix | `ambiguous option: <flag> could match <f1>, <f2>, …` |
| value missing (one value) | `argument <name>: expected one argument` |
| too few values (`+`, n) | `argument <name>: expected <min> argument(s)` |
| value attached to valueless option | `argument <name>: ignore implicit argument '<value>'` |
| mutual exclusion | `argument <name>: not allowed with argument <other name>` |
| required option missing | `argument <name> is required` (no colon) |
| required group missing | `one of the arguments <n1> <n2> … is required` |
| bad integer / long / double | §10 |
| bad enum value | `argument <name>: could not convert '<v>' (choose from {a,b})` |
| value not among `choices` | `argument <name>: invalid choice: '<v>' (choose from {a,b})` |

`<name>` is always the full textual name (`-a/--account/-u/--username`,
`-g/--group-id/--group`), never the flag the user typed.

## 9. "Did you mean" suggestions

**When.** Only for an unknown-option error whose token's body (§0.1; the body
includes any `=…` part) has at least 2 characters, and for every unknown-command
error. `-x`, `--x` and `-` never get suggestions; `-xy` does.

**Candidates.**

- Unknown option: every flag of the parser that raised the error (the subcommand
  parser for `send --bogus`, so only `send`'s flags) whose body has at least 2
  characters, i.e. all long flags; single-letter short flags are skipped. Every
  alias counts separately (`--group-id` and `--group` are separate candidates). The
  suggestion text is the full flag (`--dbus`), but distances are computed on bodies.
- Unknown command: every command name.

**Score.** For each candidate body/name `c` and the input body/token `t`: if `c`
starts with `t`, the score is 0 (so the empty token scores 0 against every command).
Otherwise the score is the weighted edit distance from `t` to `c` defined as follows.
Let d(i, j) be the cost of turning the first i characters of `t` into the first j
characters of `c`:

- d(0, j) = j and d(i, 0) = i. The borders cost 1 per character, even though a
  deletion costs 4 inside the table.
- d(i, j) is the minimum of:
  - d(i−1, j−1) + 0 if t[i] = c[j], else + 2 (substitution);
  - d(i−2, j−2) + 0, only if i ≥ 2, j ≥ 2, t[i] ≠ c[j], t[i−1] = c[j] and
    t[i] = c[j−1] (swapping two adjacent characters is free);
  - d(i−1, j) + 4 (dropping a character of `t`);
  - d(i, j−1) + 1 (adding a character of `c`).
- The score is d(len t, len c). Characters are compared as UTF-16 code units,
  case-sensitively.

**Selection and output.** Sort all candidates by (score, then candidate text in plain
string order, i.e. UTF-16 code unit order). Let m be the lowest score. If m ≥ 7, print
nothing. Otherwise print an empty line, the line `Did you mean:`, and then, for every
candidate with score m in sorted order, a TAB, the candidate text and a newline.

Checks against the reference: `bogus` scores 6 against `trust` and at least 7
against every other command (`send` 7, `block` 8, `version` 10) → `trust`;
`--bogus` → `--dbus` (`dbus` 5; `bus-name`, `help`, `username` 9, `verbose` 10);
`send --bogus` → `--group` (`group` 6, `group-id` 9); `---verbose`
(body `verbose`, a prefix of `verbose`) → `--verbose`; the empty subcommand → all 61
names in sorted order. Because sorting is by string order and not declaration order,
ties come out alphabetically (`x` → `link`, `send`).

## 10. Type conversion messages

Conversion uses the raw token text (or the attached value). It happens when the value
is taken (§7.3) or, for positionals, at distribution (§7.4).

| Declared type | Accepted text | Error message (after `argument <name>: `) |
|---|---|---|
| int (signal-cli `int.class`/`Integer`) | optional `+`/`-`, then decimal digits (any Unicode decimal digit is accepted, e.g. U+0663); value in signed 32-bit range; no spaces, no `_`, no hex | `could not convert '<v>' to integer (32 bits)` |
| long (`long.class`) | same, signed 64-bit range | `could not convert '<v>' to integer (64 bits)` |
| double (`double.class`) | Java double syntax: surrounding characters ≤ U+0020 are ignored; optional sign; decimal with optional fraction and exponent (`1e3`, `.5`, `5.`); `NaN`, `Infinity` (exact case); hexadecimal `0x1p3` (binary exponent required); optional suffix `f F d D` | `could not convert '<v>' to  number (double precision)`, with **two** spaces before `number` (the type name itself starts with a space) |
| boolean (`Boolean.class`) | anything; true if it equals `true` ignoring case, otherwise false | never fails; only a missing value is an error |
| display-string enum (`enumStringType`, e.g. `-o`, `--receive-mode`, `sendMessageRequestResponse --type`) | exact, case-sensitive match of a display string | `could not convert '<v>' (choose from {v1,v2,…})` (display strings in declaration order) |
| name-matched enum (not used by signal-cli) | exact constant name | same form with constant names |
| string with `choices` (e.g. `sendReceipt --type`, `updateGroup --link`) | one of the choices, exactly | `invalid choice: '<v>' (choose from {c1,c2,…})` |
| int with `choices` | converted first, then checked | conversion message, else `invalid choice: '<v>' (choose from {…})` |
| string (default), file | anything | — |

The ` (32 bits)`, ` (64 bits)` suffix appears for non-numeric, fractional and
out-of-range values alike. At W=75 these long messages are wrapped and justified like
any error line. Example: `signal-cli: error:  argument  -t/--timeout:  could  not  convert  'abc'  to`
/ `number (double precision)`. The doubled space before `number` falls at the line
end and is stripped by justification.

## 11. The vectors file

[`testdata/vectors/argparse_text.json`](../../../testdata/vectors/argparse_text.json),
generated by executing the jars (see its `source` block):

| Key | Content | Count |
|---|---|---|
| `line_break` | the classes and automaton of §2 in machine-readable form | 32 states |
| `segmentation.synthetic` | hand-chosen texts (error lines, URLs, numbers, every ASCII punctuation in word and number context, quotes, dashes, controls) and their segments | 32 |
| `segmentation.signal_cli_help_texts` | every help text of the real signal-cli parser (top level with `(default: …)` suffixes, every subcommand argument, every subcommand row), description and epilog | 368 |
| `segmentation.random` | seeded random ASCII strings | 400 |
| `adjust_space` | §3.2 on single lines (`line`, `width`, `cur_width` → `result`) | 24 |
| `wrap` | §3.1 on 18 texts × 12 (width, offset) combinations, plus first-indent cases | 220 |
| `help_rows` | §4 (`title`, `help`, `width` → full row text) | 153 |
| `levenshtein` | §9 distance (`input`, `candidate` → `distance`) | 190 |
| `suggestions` | real signal-cli parser: unknown commands and options (top level and subcommands), full stderr | 40 |
| `suggestions_synthetic` | parsers built from explicit flag or command lists, full stderr | 57 |
| `parsers` | 9 synthetic parsers (`definition` schema in `source.conventions`): `usage`, `help`, subcommand helps, and parse `cases` (`argv` → `exit`, `stdout`, `stderr`, `attrs`) | 169 cases |
| `known_args_logging_preparser` | §7.7: definition, help, cases with `unknown` lists | 19 |
| `signal_cli_parser` | the real signal-cli parser at W=75: argv → exit/stdout/stderr (matches the binary minus the pre-parser block and log lines) | 57 |
| `signal_cli_other_widths` | top-level help, 10 subcommand helps and 8 error cases at `COLUMNS` = 45, 65, 105 (W = 40, 60, 100) | 3 widths |

`parsers` cases emulate signal-cli's handling: help gives exit 0 and stdout, an
error gives exit 1 and stderr (usage + error line + suggestions), success gives exit
0 and `attrs`. The real binary adds the §7.7 block before help output.

## 12. Corrections to index.md

- §4 width: W is `COLUMNS` − 5 (or the terminal's columns − 5), and 75 only when
  that is ≤ 0 or unknown. `COLUMNS=80` gives 75, `COLUMNS=120` gives 115.
- §5.2 `unrecognized arguments`: for leftover positionals, the rest of the argument
  list is reported inside **one** pair of quotes (`'a b'`), not each token quoted. An
  unknown option is reported alone (parsing stops at it). Only the unknown-option
  kind gets suggestions.
- §5.2 add: `ambiguous command: <tok> could match …` (subcommand prefixes, §7.6);
  unambiguous subcommand prefixes are accepted (`vers`). Also add
  `ignore implicit argument '<v>'`, `expected <n> argument(s)`, and the double
  message's two spaces (`to  number (double precision)`; receive.md shows it
  normalised to one space).
- §5.2 "Unknown subcommand": the `invalid choice` list is in declaration order (which
  for signal-cli equals sorted order), while suggestions are always sorted.
- §1 `--`: after `--` at top level, the subcommand can no longer be chosen, and the
  error quotes all remaining tokens together (`-- version -h` →
  `'version -h'`).

## Gaps

- **Locale**: messages in German, Dutch or Russian (and CJK width counting for
  ja/zh/ko) when such a locale is the JVM default and installed. Not reproducible in
  this sandbox (no such locales installed); only English is specified, and the
  translated texts were not recorded.
- **Non-ASCII line breaking** is specified only for the characters listed in §2.4.
  No-break spaces, zero-width and combining marks, CJK, Thai and Indic text would
  segment differently from any approximation. They only matter inside wrapped error
  lines that echo such user input.
- **Terminal width detection** without `COLUMNS` uses `stty` on `/dev/tty`. Exact
  equivalence (e.g. when `/dev/tty` exists but `stty` reports nothing) was not probed.
- **Upper-case metavars** use the JVM default locale's upper-casing (a Turkish locale
  would turn `i` into `İ`); signal-cli's dests are ASCII, so English rules suffice.
- **Not covered** because signal-cli does not use them: `fromfile` prefixes (`@file`
  argument files and their "trailing white spaces" hint), aliases, custom usage
  strings, separately titled argument groups (only outlined in §6.1), `append`/`const`
  actions, and nargs `?` const values other than null.
