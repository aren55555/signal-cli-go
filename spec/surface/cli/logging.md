# signal-cli logging (stderr and `--log-file`)

Refines [index.md](index.md) §8. Everything here concerns diagnostic log output. It never
goes to stdout, with one exception: logback's own status dump when the log file cannot be
opened (§2.6). Command output (stdout) is never affected by verbosity, `--log-file` or
`--scrub-log`.

## Sources

- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/logging/LogConfigurator.java (tag v0.14.7): logger levels, appenders, layouts, console filter
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/logging/Scrubber.java (tag v0.14.7): `--scrub-log` rules
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/logging/ScrubberPatternLayout.java (tag v0.14.7): where scrubbing is applied
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/Main.java (tag v0.14.7): early logging pass, JUL bridge, libsignal log bridges, config-file defaults
- https://github.com/AsamK/signal-cli/blob/v0.14.7/src/main/java/org/asamk/signal/GlobalConfig.java and `ConfigLoader.java` (tag v0.14.7): `verbose`, `logFile`, `scrubLog` config keys
- https://github.com/AsamK/signal-cli/blob/v0.14.7/lib/src/main/java/org/asamk/signal/manager/internal/LibSignalLogger.java and `SignalLogger.java` (tag v0.14.7): string-named loggers `LibSignal`, `LibSignalService`
- Every `LoggerFactory.getLogger(...)` call in `src/main/java` and `lib/src/main/java` at tag v0.14.7 (432 files, all scanned) for §4
- https://github.com/AsamK/signal-cli/blob/v0.14.7/gradle/libs.versions.toml: logback-classic 1.6.1, slf4j 2.0.18; the bundled binary ships `logback-classic-1.6.1.jar`, `logback-core-1.6.1.jar`
- logback tag v_1.6.1 (https://github.com/qos-ch/logback/tree/v_1.6.1):
  `logback-classic/.../pattern/TargetLengthBasedClassNameAbbreviator.java`, `ClassNameOnlyAbbreviator.java`,
  `NamedConverter.java`, `DateConverter.java`, `EnsureExceptionHandling.java`, `ThrowableProxyConverter.java`,
  `LineSeparatorConverter.java`, `ThreadConverter.java`, `spi/ThrowableProxyUtil.java`, `spi/StackTraceElementProxy.java`;
  `logback-classic/.../util/LoggerNameUtil.java` (hierarchy separators `.` and `$`);
  `logback-core/.../util/CachingDateFormatter.java`, `FileAppender.java`, `CoreConstants.java`
- Binary observation (bundled signal-cli 0.14.7, `LANG=C.UTF-8`, scratch `--config` dirs under `~/.cache`, run 2026-10-29):
  `-v --version`; `-vv version`; `-vvv version`; `-vvv --scrub-log version`; `-vv --scrub-log version`;
  `-vv --scrub-log --log-file F version`; `--log-file F version` (F pre-existing, and F in a
  non-existent subdirectory); `-v --log-file F version` (F pre-existing); `-v listAccounts` and
  `-v --log-file F listAccounts` on an empty config dir; `-v --log-file /proc/nope/x.log version`;
  `TZ=Asia/Kolkata -v --version`, `TZ=America/New_York -v --version`; `SIGNAL_CLI_CONFIG=<json>`
  with `{"verbose":1}` (with and without `-v`) and `{"scrubLog":true,"logFile":...}`.

## 1. Inputs

Three settings control logging. They come from the early pass over the whole argument
list described in index.md §8 and argparse.md (the "second, minimal parser"):

- verbosity V: the number of `-v`/`--verbose` occurrences;
- log file F: the `--log-file` value (the last one wins), or none;
- scrub S: `--scrub-log` present.

Defaults come from the merged JSON config file (keys `verbose` (integer), `logFile`
(string path), `scrubLog` (boolean)). The `-v` count is **added** to the config value
(observed: config `{"verbose":1}` alone behaves as `-v`; with one `-v` on the command line
it behaves as `-vv`). `--log-file` overrides the config path, and `--scrub-log` can only
switch scrubbing on. If the early pass fails (e.g. `--log-file` with no value), the
config values (or 0/none/off) are used.

Logging is configured once, before any command runs. The log file is opened then, even if
no event is ever written.

## 2. Line formats

### 2.1 Simple format

Used for the console when V = 0, and for the console whenever F is given (any V):

```
<LEVEL><pad> <NAME> - <MESSAGE><EOL>[<STACKTRACE>]
```

- `<LEVEL>` is one of `ERROR`, `WARN`, `INFO`, `DEBUG`, `TRACE`, left-aligned and
  right-padded with spaces to 5 characters. `WARN` and `INFO` are therefore followed by two
  spaces before the logger name: `WARN  MultiAccountManager - ...`.
- `<NAME>` is the logger name cut down to the part after the **last** `.` (the whole name
  if it has no `.`). A `$` is not a separator here: a logger named after a nested class
  `a.b.Outer$Inner` shows as `Outer$Inner`. String-named loggers without dots
  (`LibSignal`, `LibSignalService`) are shown in full.
- ` - ` is space, hyphen-minus, space.
- `<MESSAGE>` is the formatted message, emitted verbatim (it may contain newlines).
- `<EOL>` is the platform line separator: `\n` on Linux/macOS (`\r\n` on Windows).

Example (observed): `WARN  MultiAccountManager - Ignoring +15555550111: User is not registered. (NotRegisteredException)`

### 2.2 Detailed format

Used for the console when V ≥ 1 and no F is given, and **always** for the log file:

```
<TIMESTAMP> [<THREAD>] <LEVEL><pad> <ABBREVIATED NAME> - <MESSAGE><EOL>[<STACKTRACE>]
```

- `<TIMESTAMP>`: `YYYY-MM-DDTHH:MM:SS.mmm` followed by the zone offset. The time is the
  event time in the **process's local time zone** (the `TZ` environment variable or the
  system zone; not forced to UTC). The offset is the letter `Z` when it is zero, otherwise
  a sign and four digits with no colon (`+0530`, `-0400`). Milliseconds are always 3
  digits. All fields are zero-padded ASCII digits; the year is 4 digits. Observed:
  `2026-10-29T09:22:29.953Z` (UTC host), `2026-10-29T14:52:46.366+0530`
  (`TZ=Asia/Kolkata`), `2026-10-29T05:22:48.219-0400` (`TZ=America/New_York`).
- `[<THREAD>]`: the name of the thread that logged, in square brackets, verbatim. The main
  thread is `main`. Other names are JVM/executor thread names. The Go port should use
  `main` for the main flow and stable descriptive names elsewhere; they are not part of
  the contract.
- `<LEVEL><pad>`: as in 2.1.
- `<ABBREVIATED NAME>`: the full logger name shortened to a target length of 36 (§3).
- Remaining fields as in 2.1.

Examples (observed):

```
2026-10-29T09:22:29.953Z [main] DEBUG org.asamk.signal.util.IOUtils - XDG_RUNTIME_DIR not set, falling back to temp dir
2026-10-29T09:22:31.735Z [main] DEBUG org.asamk.signal.App - Starting signal-cli 0.14.7
2026-10-29T09:22:32.202Z [main] TRACE org.asamk.signal.manager.Manager - Expected exception when checking libsignal-client: Failed to deserialize zkgroup::api::groups::uuid_ciphertext::UuidCiphertext
```

### 2.3 Attached exceptions (both formats)

If the event carries an exception, its stack trace comes right after the `<EOL>` of the
line, in both formats. Neither layout names a throwable field, so logback adds the plain
stack-trace renderer. Packaging data is off, so frames get no `[jar:version]` suffix.
Layout:

- First line: the exception class's fully qualified name, then `: ` and the message if
  the message is non-null. Then EOL.
- One line per stack frame: a TAB, `at `, the frame text, EOL. Observed:
  `\tat java.base/java.lang.ProcessBuilder.start(ProcessBuilder.java:1099)`.
- If frames are shared with the enclosing trace: TAB, `... <n> common frames omitted`, EOL.
- Suppressed exceptions come before the cause, recursively, with one more TAB of
  indentation. Their first line is (indent−1) TABs + `Suppressed: ` + class/message.
- The cause follows as `Caused by: <class>: <message>` at the same indentation as its
  parent's first line, then its own frames. Cyclic causes print as
  `[CIRCULAR REFERENCE: <class>: <message>]`.

Java frame text cannot be reproduced exactly by a Go port. Only the shape (first line,
TAB-indented `at` lines, `Caused by: `) is meaningful. Observed at `-vvv`: the JDK's own
`java.lang.ProcessBuilder` and `java.lang.Runtime` debug events carry such traces
(`java.lang.RuntimeException: ProcessBuilder.start() debug`,
`java.lang.Throwable: Runtime.exit(0)`). These are JVM artifacts and need not be
reproduced.

### 2.4 Encoding, buffering, separators

Each event is written as one unit (formatted line plus any trace), UTF-8 encoded, and
flushed right away, on both the console and the file. The line separator is the
platform's (`\n` on Linux) in both destinations, including inside stack traces.

### 2.5 Destinations

- The console is **stderr**.
- The file is opened in **append** mode: an existing file is kept and new lines go after
  it (observed: a pre-existing line `preexisting` survived, followed by new lines).
  Missing parent directories are created (observed: `--log-file <dir>/sub/n.log` created
  `sub/` and an empty `n.log`). The file is created (empty) even when nothing is logged.
  The path is resolved to an absolute path against the working directory.

### 2.6 Unopenable log file (observed quirk)

If the file cannot be opened (e.g. `/proc/nope/x.log`), logging carries on with the
console only (same console format/filter as with F, see §5). Logback prints its internal
status report to **stdout** before the command's own output: about 40 lines starting
with `HH:MM:SS,mmm |-INFO in ch.qos.logback.classic.LoggerContext[default] - Found
logback-core version 1.6.1`, including `|-ERROR` lines for the failed open with a Java
stack trace. The exit status is unaffected (0 for `version`). This report is a JVM/logback
artifact and is not part of the compatibility contract. A Go port should print a single
warning to stderr instead and should not write to stdout.

## 3. Logger-name abbreviation (detailed format, target length T = 36)

Input: the full logger name, length N.

1. If N < 36, the name is used unchanged. (N = 36 is **not** exempt; see the
   `DbusSignalImpl` example.)
2. If the name contains no `.`, it is used unchanged, whatever its length. Only `.` counts
   as a separator here; `$` does not.
3. Split at the **last** `.`: the *tail* is that dot plus the final segment, length L.
   The *prefix* is everything before that dot, length P = N − L.
4. Prefix budget B = max(0, 36 − L). Required trim R = P − B (how many prefix characters
   need to go).
5. Go through the prefix segments (the dot-separated pieces of the prefix) from left to
   right, keeping a running count of characters removed (starts at 0):
   - The **first** segment is always reduced to its first character (count += its
     length − 1).
   - Before each later prefix segment, compare the count with R. If count ≥ R, stop: the
     rest of the name from the dot before this segment (including all remaining prefix
     segments and the tail) is copied verbatim. Otherwise keep the dot and reduce this
     segment to its first character (count += length − 1).
   - If every prefix segment was reduced, append the tail (no check at the last dot).
6. Dots between processed segments are kept. Empty segments (consecutive dots) contribute
   nothing but their dot.

The result can still be longer than 36 (a long final segment is never shortened). It can
also be shorter than necessary, because segments shrink to one character as a whole.

Worked examples:

| N | Full name | Result |
|---|---|---|
| 20 | `org.asamk.signal.App` | `org.asamk.signal.App` (N < 36) |
| 29 | `org.asamk.signal.util.IOUtils` | unchanged (observed) |
| 35 | `org.asamk.signal.manager.util.Utils` | unchanged |
| 36 | `org.asamk.signal.dbus.DbusSignalImpl` | `o.asamk.signal.dbus.DbusSignalImpl` (L=15, B=21, P=21, R=0: first segment reduced, then stop) |
| 38 | `org.asamk.signal.commands.BlockCommand` | `o.asamk.signal.commands.BlockCommand` |
| 45 | `org.asamk.signal.manager.helper.ReceiveHelper` | `o.a.s.manager.helper.ReceiveHelper` |
| 56 | `org.asamk.signal.manager.helper.UnidentifiedAccessHelper` | `o.a.s.m.h.UnidentifiedAccessHelper` |
| 56 | `org.asamk.signal.manager.storage.groups.LegacyGroupStore` | `o.a.s.m.s.groups.LegacyGroupStore` |
| 57 | `org.asamk.signal.manager.internal.ProvisioningManagerImpl` | `o.a.s.m.i.ProvisioningManagerImpl` |

Step by step for `org.asamk.signal.manager.helper.UnidentifiedAccessHelper` (N=56): tail
`.UnidentifiedAccessHelper` L=25, B=11, P=31, R=20. `org`→`o` (count 2), 2<20 so
`asamk`→`a` (6), 6<20 so `signal`→`s` (11), 11<20 so `manager`→`m` (17), 17<20 so
`helper`→`h` (22). Prefix done, tail appended: `o.a.s.m.h.UnidentifiedAccessHelper` (34
chars).

Step by step for `org.asamk.signal.manager.storage.groups.LegacyGroupStore` (N=56): L=17,
B=19, P=39, R=20. `o`(2) `a`(6) `s`(11) `m`(17) `s`(23). Before `groups`, 23 ≥ 20, so
stop and copy `.groups.LegacyGroupStore` verbatim: `o.a.s.m.s.groups.LegacyGroupStore`.

Step by step for `org.asamk.signal.manager.helper.ReceiveHelper` (N=45): L=14, B=22, P=31,
R=9. `o`(2) `a`(6) `s`(11). Before `manager`, 11 ≥ 9, so stop:
`o.a.s.manager.helper.ReceiveHelper`.

§4 lists the abbreviated form of every signal-cli logger.

## 4. Logger names

Loggers are named with the fully qualified Java class name (`Class.getName()`), except
where noted. The simple format prints the part after the last `.`. The detailed format
prints the §3 abbreviation.

### 4.1 Requested short names

| Short name | Fully qualified logger name | Detailed-format (36) form | Note |
|---|---|---|---|
| ReceiveHelper | `org.asamk.signal.manager.helper.ReceiveHelper` | `o.a.s.manager.helper.ReceiveHelper` | |
| ProvisioningManagerImpl | `org.asamk.signal.manager.internal.ProvisioningManagerImpl` | `o.a.s.m.i.ProvisioningManagerImpl` | |
| SyncStorageJob | `org.asamk.signal.manager.jobs.SyncStorageJob` | `o.a.s.manager.jobs.SyncStorageJob` | |
| SyncHelper | `org.asamk.signal.manager.helper.SyncHelper` | `o.a.signal.manager.helper.SyncHelper` | |
| StorageHelper | `org.asamk.signal.manager.helper.StorageHelper` | `o.a.s.manager.helper.StorageHelper` | |
| MultiAccountManager | `org.asamk.signal.manager.MultiAccountManager` | `o.a.s.manager.MultiAccountManager` | **Named differently from its class:** the logging class is `org.asamk.signal.manager.SignalAccountFiles`, which logs under the `MultiAccountManager` interface name. (The class `org.asamk.signal.manager.internal.MultiAccountManagerImpl` has its own logger, short name `MultiAccountManagerImpl`, detailed `o.a.s.m.i.MultiAccountManagerImpl`.) |
| HttpServerHandler | `org.asamk.signal.http.HttpServerHandler` | `o.a.signal.http.HttpServerHandler` | |
| DaemonCommand | `org.asamk.signal.commands.DaemonCommand` | `o.a.signal.commands.DaemonCommand` | |
| AddStickerPackCommand | `org.asamk.signal.commands.AddStickerPackCommand` | `o.a.s.commands.AddStickerPackCommand` | |
| StickerHelper | `org.asamk.signal.manager.helper.StickerHelper` | `o.a.s.manager.helper.StickerHelper` | |
| SendHelper | `org.asamk.signal.manager.helper.SendHelper` | `o.a.signal.manager.helper.SendHelper` | |
| ProfileHelper | `org.asamk.signal.manager.helper.ProfileHelper` | `o.a.s.manager.helper.ProfileHelper` | |
| PinHelper | `org.asamk.signal.manager.helper.PinHelper` | `o.a.signal.manager.helper.PinHelper` | |
| ManagerImpl | `org.asamk.signal.manager.internal.ManagerImpl` | `o.a.s.manager.internal.ManagerImpl` | |
| GroupHelper | `org.asamk.signal.manager.helper.GroupHelper` | `o.a.s.manager.helper.GroupHelper` | |
| BlockCommand | `org.asamk.signal.commands.BlockCommand` | `o.asamk.signal.commands.BlockCommand` | |
| AttachmentHelper | `org.asamk.signal.manager.helper.AttachmentHelper` | `o.a.s.m.helper.AttachmentHelper` | |
| AccountHelper | `org.asamk.signal.manager.helper.AccountHelper` | `o.a.s.manager.helper.AccountHelper` | |
| Shutdown | `org.asamk.signal.Shutdown` | `org.asamk.signal.Shutdown` | |
| RegistrationManagerImpl | `org.asamk.signal.manager.internal.RegistrationManagerImpl` | `o.a.s.m.i.RegistrationManagerImpl` | |
| AddDeviceCommand | `org.asamk.signal.commands.AddDeviceCommand` | `o.a.signal.commands.AddDeviceCommand` | |
| RegistrationManager | none | none | **No logger with this name exists in 0.14.7.** `org.asamk.signal.manager.RegistrationManager` is an interface with no logger, nothing logs under its name, and no bundled jar contains a string-named `RegistrationManager` logger. Registration-flow lines come from `RegistrationManagerImpl` (above). If an observation shows `RegistrationManager - ...`, it is the truncated name `RegistrationManagerImpl` or comes from another version. |
| IOUtils | `org.asamk.signal.util.IOUtils` | `org.asamk.signal.util.IOUtils` (observed) | The CLI-side utility class. The library's `org.asamk.signal.manager.util.IOUtils` has no logger. |
| App | `org.asamk.signal.App` | `org.asamk.signal.App` (observed) | With `--scrub-log` in the detailed format this prints as `***.App` (§6.4). |

### 4.2 Other names worth knowing

| Short / printed name | Logger name | Note |
|---|---|---|
| `LibSignal` | `LibSignal` (string name) | libsignal-client protocol log bridge. Message is `[<tag>]: <message>`. Wired up only when V ≥ 1. |
| `LibSignalService` | `LibSignalService` (string name) | signal-service (network library) log bridge. Message is `[<tag>]: <message>`. Wired up only when V ≥ 1. |
| `Manager` | `org.asamk.signal.manager.Manager` | Used by a static check on the interface. Observed at `-vv`: `TRACE ... Expected exception when checking libsignal-client: ...` |
| `SignalJsonRpcDispatcherHandler` | `org.asamk.signal.jsonrpc.SignalJsonRpcDispatcherHandler` | **Also used by `SignalJsonRpcCommandHandler`** (named differently from that class). Detailed: `o.a.s.j.SignalJsonRpcDispatcherHandler`. |
| JDK/JUL names, e.g. `java.lang.ProcessBuilder`, `java.lang.Runtime` | as given by java.util.logging | JUL records are forwarded with their JUL logger name. Levels map FINEST→TRACE, FINER/FINE→DEBUG, INFO→INFO, WARNING→WARN, SEVERE→ERROR. |

### 4.3 Complete list of signal-cli class-named loggers and their 36-char forms

Every `getLogger(<Class>)` in 0.14.7. Names shorter than 36 are unchanged.

```
org.asamk.signal.App                                         org.asamk.signal.App
org.asamk.signal.Shutdown                                    org.asamk.signal.Shutdown
org.asamk.signal.commands.AddDeviceCommand                   o.a.signal.commands.AddDeviceCommand
org.asamk.signal.commands.AddStickerPackCommand              o.a.s.commands.AddStickerPackCommand
org.asamk.signal.commands.BlockCommand                       o.asamk.signal.commands.BlockCommand
org.asamk.signal.commands.DaemonCommand                      o.a.signal.commands.DaemonCommand
org.asamk.signal.commands.FinishLinkCommand                  o.a.s.commands.FinishLinkCommand
org.asamk.signal.commands.GetUserStatusCommand               o.a.s.commands.GetUserStatusCommand
org.asamk.signal.commands.JsonRpcDispatcherCommand           o.a.s.c.JsonRpcDispatcherCommand
org.asamk.signal.commands.LinkCommand                        o.asamk.signal.commands.LinkCommand
org.asamk.signal.commands.ListDevicesCommand                 o.a.s.commands.ListDevicesCommand
org.asamk.signal.commands.ListGroupsCommand                  o.a.s.commands.ListGroupsCommand
org.asamk.signal.commands.ListIdentitiesCommand              o.a.s.commands.ListIdentitiesCommand
org.asamk.signal.commands.QuitGroupCommand                   o.a.signal.commands.QuitGroupCommand
org.asamk.signal.commands.ReceiveCommand                     o.a.signal.commands.ReceiveCommand
org.asamk.signal.commands.SendCommand                        o.asamk.signal.commands.SendCommand
org.asamk.signal.commands.SendPollCreateCommand              o.a.s.commands.SendPollCreateCommand
org.asamk.signal.commands.SendPollTerminateCommand           o.a.s.c.SendPollTerminateCommand
org.asamk.signal.commands.SendPollVoteCommand                o.a.s.commands.SendPollVoteCommand
org.asamk.signal.commands.StartLinkCommand                   o.a.signal.commands.StartLinkCommand
org.asamk.signal.commands.UnblockCommand                     o.a.signal.commands.UnblockCommand
org.asamk.signal.commands.UpdateGroupCommand                 o.a.s.commands.UpdateGroupCommand
org.asamk.signal.commands.UploadStickerPackCommand           o.a.s.c.UploadStickerPackCommand
org.asamk.signal.commands.VerifyCommand                      o.a.signal.commands.VerifyCommand
org.asamk.signal.dbus.DbusHandler                            org.asamk.signal.dbus.DbusHandler
org.asamk.signal.dbus.DbusSignalControlImpl                  o.a.s.dbus.DbusSignalControlImpl
org.asamk.signal.dbus.DbusSignalImpl                         o.asamk.signal.dbus.DbusSignalImpl
org.asamk.signal.http.HttpServerHandler                      o.a.signal.http.HttpServerHandler
org.asamk.signal.json.JsonReceiveMessageHandler              o.a.s.json.JsonReceiveMessageHandler
org.asamk.signal.jsonrpc.JsonRpcReader                       o.asamk.signal.jsonrpc.JsonRpcReader
org.asamk.signal.jsonrpc.SignalJsonRpcDispatcherHandler      o.a.s.j.SignalJsonRpcDispatcherHandler
org.asamk.signal.jsonrpc.SocketHandler                       o.asamk.signal.jsonrpc.SocketHandler
org.asamk.signal.manager.Manager                             org.asamk.signal.manager.Manager
org.asamk.signal.manager.MultiAccountManager                 o.a.s.manager.MultiAccountManager
org.asamk.signal.manager.api.RecipientIdentifier             o.a.s.m.api.RecipientIdentifier
org.asamk.signal.manager.helper.AccountHelper                o.a.s.manager.helper.AccountHelper
org.asamk.signal.manager.helper.AttachmentHelper             o.a.s.m.helper.AttachmentHelper
org.asamk.signal.manager.helper.CallManager                  o.a.s.manager.helper.CallManager
org.asamk.signal.manager.helper.GroupHelper                  o.a.s.manager.helper.GroupHelper
org.asamk.signal.manager.helper.GroupV2Helper                o.a.s.manager.helper.GroupV2Helper
org.asamk.signal.manager.helper.IdentityHelper               o.a.s.manager.helper.IdentityHelper
org.asamk.signal.manager.helper.IncomingMessageHandler       o.a.s.m.h.IncomingMessageHandler
org.asamk.signal.manager.helper.PinHelper                    o.a.signal.manager.helper.PinHelper
org.asamk.signal.manager.helper.PreKeyHelper                 o.a.s.manager.helper.PreKeyHelper
org.asamk.signal.manager.helper.ProfileHelper                o.a.s.manager.helper.ProfileHelper
org.asamk.signal.manager.helper.ReceiveHelper                o.a.s.manager.helper.ReceiveHelper
org.asamk.signal.manager.helper.RecipientHelper              o.a.s.manager.helper.RecipientHelper
org.asamk.signal.manager.helper.SendHelper                   o.a.signal.manager.helper.SendHelper
org.asamk.signal.manager.helper.StickerHelper                o.a.s.manager.helper.StickerHelper
org.asamk.signal.manager.helper.StorageHelper                o.a.s.manager.helper.StorageHelper
org.asamk.signal.manager.helper.SyncHelper                   o.a.signal.manager.helper.SyncHelper
org.asamk.signal.manager.helper.UnidentifiedAccessHelper     o.a.s.m.h.UnidentifiedAccessHelper
org.asamk.signal.manager.internal.JobExecutor                o.a.s.manager.internal.JobExecutor
org.asamk.signal.manager.internal.ManagerImpl                o.a.s.manager.internal.ManagerImpl
org.asamk.signal.manager.internal.MultiAccountManagerImpl    o.a.s.m.i.MultiAccountManagerImpl
org.asamk.signal.manager.internal.ProvisioningManagerImpl    o.a.s.m.i.ProvisioningManagerImpl
org.asamk.signal.manager.internal.RegistrationManagerImpl    o.a.s.m.i.RegistrationManagerImpl
org.asamk.signal.manager.internal.SignalDependencies         o.a.s.m.internal.SignalDependencies
org.asamk.signal.manager.internal.SignalWebSocketHealthMonitor  o.a.s.m.i.SignalWebSocketHealthMonitor
org.asamk.signal.manager.jobs.CheckWhoAmIJob                 o.a.s.manager.jobs.CheckWhoAmIJob
org.asamk.signal.manager.jobs.CleanOldPreKeysJob             o.a.s.m.jobs.CleanOldPreKeysJob
org.asamk.signal.manager.jobs.DownloadProfileAvatarJob       o.a.s.m.j.DownloadProfileAvatarJob
org.asamk.signal.manager.jobs.DownloadProfileJob             o.a.s.m.jobs.DownloadProfileJob
org.asamk.signal.manager.jobs.RefreshRecipientsJob           o.a.s.m.jobs.RefreshRecipientsJob
org.asamk.signal.manager.jobs.RetrieveStickerPackJob         o.a.s.m.jobs.RetrieveStickerPackJob
org.asamk.signal.manager.jobs.SyncStorageJob                 o.a.s.manager.jobs.SyncStorageJob
org.asamk.signal.manager.storage.AccountDatabase             o.a.s.m.storage.AccountDatabase
org.asamk.signal.manager.storage.SignalAccount               o.a.s.manager.storage.SignalAccount
org.asamk.signal.manager.storage.Utils                       o.asamk.signal.manager.storage.Utils
org.asamk.signal.manager.storage.accounts.AccountsStore      o.a.s.m.s.accounts.AccountsStore
org.asamk.signal.manager.storage.groups.GroupStore           o.a.s.m.storage.groups.GroupStore
org.asamk.signal.manager.storage.groups.LegacyGroupStore     o.a.s.m.s.groups.LegacyGroupStore
org.asamk.signal.manager.storage.identities.IdentityKeyStore o.a.s.m.s.i.IdentityKeyStore
org.asamk.signal.manager.storage.identities.LegacyIdentityKeyStore  o.a.s.m.s.i.LegacyIdentityKeyStore
org.asamk.signal.manager.storage.keyValue.KeyValueStore      o.a.s.m.s.keyValue.KeyValueStore
org.asamk.signal.manager.storage.messageCache.CachedMessage  o.a.s.m.s.messageCache.CachedMessage
org.asamk.signal.manager.storage.messageCache.MessageCache   o.a.s.m.s.messageCache.MessageCache
org.asamk.signal.manager.storage.prekeys.KyberPreKeyStore    o.a.s.m.s.prekeys.KyberPreKeyStore
org.asamk.signal.manager.storage.prekeys.LegacyPreKeyStore   o.a.s.m.s.prekeys.LegacyPreKeyStore
org.asamk.signal.manager.storage.prekeys.LegacySignedPreKeyStore  o.a.s.m.s.p.LegacySignedPreKeyStore
org.asamk.signal.manager.storage.prekeys.PreKeyStore         o.a.s.m.storage.prekeys.PreKeyStore
org.asamk.signal.manager.storage.prekeys.SignedPreKeyStore   o.a.s.m.s.prekeys.SignedPreKeyStore
org.asamk.signal.manager.storage.protocol.LegacyJsonIdentityKeyStore  o.a.s.m.s.p.LegacyJsonIdentityKeyStore
org.asamk.signal.manager.storage.recipients.LegacyRecipientStore2  o.a.s.m.s.r.LegacyRecipientStore2
org.asamk.signal.manager.storage.recipients.MergeRecipientHelper  o.a.s.m.s.r.MergeRecipientHelper
org.asamk.signal.manager.storage.recipients.RecipientStore   o.a.s.m.s.recipients.RecipientStore
org.asamk.signal.manager.storage.sendLog.MessageSendLogStore o.a.s.m.s.s.MessageSendLogStore
org.asamk.signal.manager.storage.senderKeys.LegacySenderKeyRecordStore  o.a.s.m.s.s.LegacySenderKeyRecordStore
org.asamk.signal.manager.storage.senderKeys.LegacySenderKeySharedStore  o.a.s.m.s.s.LegacySenderKeySharedStore
org.asamk.signal.manager.storage.senderKeys.SenderKeyRecordStore  o.a.s.m.s.s.SenderKeyRecordStore
org.asamk.signal.manager.storage.senderKeys.SenderKeySharedStore  o.a.s.m.s.s.SenderKeySharedStore
org.asamk.signal.manager.storage.sessions.LegacySessionStore o.a.s.m.s.s.LegacySessionStore
org.asamk.signal.manager.storage.sessions.SessionStore       o.a.s.m.s.sessions.SessionStore
org.asamk.signal.manager.storage.stickers.StickerStore       o.a.s.m.s.stickers.StickerStore
org.asamk.signal.manager.syncStorage.AccountRecordProcessor  o.a.s.m.s.AccountRecordProcessor
org.asamk.signal.manager.syncStorage.ContactRecordProcessor  o.a.s.m.s.ContactRecordProcessor
org.asamk.signal.manager.syncStorage.DefaultStorageRecordProcessor  o.a.s.m.s.DefaultStorageRecordProcessor
org.asamk.signal.manager.syncStorage.GroupV1RecordProcessor  o.a.s.m.s.GroupV1RecordProcessor
org.asamk.signal.manager.syncStorage.GroupV2RecordProcessor  o.a.s.m.s.GroupV2RecordProcessor
org.asamk.signal.manager.syncStorage.StorageSyncValidations  o.a.s.m.s.StorageSyncValidations
org.asamk.signal.manager.util.AttachmentUtils                o.a.s.manager.util.AttachmentUtils
org.asamk.signal.manager.util.MessageCacheUtils              o.a.s.manager.util.MessageCacheUtils
org.asamk.signal.manager.util.NumberVerificationUtils        o.a.s.m.util.NumberVerificationUtils
org.asamk.signal.manager.util.PaymentUtils                   o.a.signal.manager.util.PaymentUtils
org.asamk.signal.manager.util.PhoneNumberFormatter           o.a.s.m.util.PhoneNumberFormatter
org.asamk.signal.manager.util.ProfileUtils                   o.a.signal.manager.util.ProfileUtils
org.asamk.signal.manager.util.Utils                          org.asamk.signal.manager.util.Utils
org.asamk.signal.util.IOUtils                                org.asamk.signal.util.IOUtils
org.asamk.signal.util.Util                                   org.asamk.signal.util.Util
```

## 5. Which events appear where

### 5.1 Logger levels

Logger levels are inherited through the dot-separated hierarchy. The `$` of nested class
names also counts as a hierarchy separator. A logger takes the level of its nearest
configured ancestor. Five loggers are configured. All others, including `LibSignal`,
`LibSignalService` and every JDK/third-party name, inherit from the root.

| Configured logger | V=0 | V=1 | V=2 | V=3 | V≥4 |
|---|---|---|---|---|---|
| root (everything not below) | WARN | INFO | INFO | TRACE | TRACE |
| `org.asamk` (all signal-cli loggers: `org.asamk.signal...`) | INFO | DEBUG | TRACE | TRACE | TRACE |
| `org.asamk.Signal` (capital S: the D-Bus interface namespace; does **not** cover `org.asamk.signal...`) | WARN | WARN | INFO | TRACE | TRACE |
| `com.zaxxer.hikari.pool.PoolBase` | WARN | WARN | INFO | TRACE | TRACE |
| `org.sqlite.core.NativeDB` | WARN | WARN | INFO | INFO | TRACE |

An event is emitted if its level is at or above the logger's effective level (ordering
TRACE < DEBUG < INFO < WARN < ERROR). Only then do the appenders below see it.

The bridges add sources depending on V:

- `LibSignal` and `LibSignalService` are connected only when V ≥ 1. At V = 0 these
  libraries produce no log output at all.
- java.util.logging is always forwarded, with JUL's own threshold at INFO for V = 1–2 and
  at FINEST for V ≥ 3. In practice the logback levels above decide. Observed: JDK
  `java.lang.ProcessBuilder`/`java.lang.Runtime` DEBUG events appear only at `-vvv`.

### 5.2 Console and file per case

| Case | Console (stderr) format | Console content | File format | File content |
|---|---|---|---|---|
| V=0, no F | simple | all emitted events (WARN+ from anything, INFO+ from `org.asamk...`) | — | — |
| V≥1, no F | detailed | all emitted events | — | — |
| V=0, F | simple | events passing the console filter (below) | detailed | all emitted events. Same set as the console at V=0 (the filter removes nothing extra, because LibSignal* are not connected), but in the detailed format |
| V≥1, F | simple | events passing the console filter | detailed | all emitted events (DEBUG/TRACE from signal-cli, INFO+/TRACE from others, LibSignal*, ...) |

Console filter (only when F is given): an event goes to the console if **both** hold:

1. its logger name is not exactly `LibSignal` and not exactly `LibSignalService`; these go
   only to the file, **even at WARN/ERROR**; and
2. its level is WARN or ERROR, **or** its level is INFO and the logger name starts with
   the string `org.asamk` (a plain string-prefix test).

Observed: `-v --log-file F version` prints nothing to stderr and appends to F:

```
2026-10-29T09:22:52.362Z [main] DEBUG org.asamk.signal.util.IOUtils - XDG_RUNTIME_DIR not set, falling back to temp dir
2026-10-29T09:22:52.371Z [main] DEBUG org.asamk.signal.App - Starting signal-cli 0.14.7
```

`--log-file F version` (V=0) leaves F unchanged (created empty if missing), because
nothing at INFO+ is logged.

Observed lines per verbosity for `version` (no account, empty config, UTC host):

- `-v --version`: one line, IOUtils DEBUG. `--version` exits during parsing, before App
  logs.
- `-v version` / `-v listAccounts`: IOUtils DEBUG and `App - Starting signal-cli 0.14.7` DEBUG.
- `-vv version`: the above plus `TRACE org.asamk.signal.manager.Manager - Expected exception when checking libsignal-client: Failed to deserialize zkgroup::api::groups::uuid_ciphertext::UuidCiphertext`.
- `-vvv version`: the above plus the JDK debug events with stack traces (§2.3), before
  and after.

### 5.3 Not part of logging

Command error messages (`CommandException` texts, index.md exit codes) are written
straight to stderr, not through a logger. When V > 0 and the error has a cause, the cause's
Java stack trace is printed after the message. Such output has no level/logger prefix,
does not go to the log file, and is never scrubbed. The same holds for argparse errors and
help.

## 6. `--scrub-log`

### 6.1 Scope

When S is on, both the console layout and the file layout scrub their output. Scrubbing
runs on the **complete formatted text of each event**: timestamp, thread, level, the
printed (possibly abbreviated) logger name, message, the trailing line separator, and any
stack trace. It applies to every event that is written, on both destinations, at every
verbosity. Stdout, the direct stderr output of §5.3 and logback's status dump (§2.6) are
not scrubbed.

### 6.2 Rule order

The rules below run one after another on the event text, in this order, each on the
result of the previous one:

1. UUIDs
2. phone numbers (E.164)
3. e-mail addresses
4. group ids, v2 form
5. group ids, v1 form
6. domain names
7. IPv4 addresses

Each rule scans left to right for non-overlapping matches. The search resumes right after
the end of each match. Replacements are not re-scanned by the same rule, but later rules
see them.

Character-class conventions (Java regex defaults, no Unicode classes): "digit" means ASCII
`0`–`9` only. "Whitespace" means space, TAB, LF, VT (0x0B), FF, CR. "Word character" means
ASCII letters, digits and `_`. A "word boundary" is a position where exactly one of the
neighbouring characters (before/after) is a word character; text start and end count as
non-word. "Base64ish character" means ASCII letter, digit, `/`, `_`, `+` or `-`. Matching
is case-sensitive unless stated.

### 6.3 Rules

**1. UUID** (case-insensitive hex): 8 hex digits, separator, 4 hex, separator, 4 hex,
separator, 4 hex, separator, 12 hex. Each separator is independently `-` or `_`. There is no
boundary requirement, so this can match inside a longer hex run: the first such window from
the left is used and extra hex characters stay. The match also takes an optional literal
`JOB::` directly before the UUID.
- With the `JOB::` prefix: the text is left unchanged (job identifiers are exempt).
- Otherwise: replaced by `********-****-****-****-**********` (8,4,4,4,10 stars with
  hyphens) followed by the UUID's **last 2 characters**, original case kept. Underscore
  separators become hyphens.

**2. Phone number**: a prefix that is `+`, or the three characters `%2B` (uppercase `B`
only; `%2b` is not matched), or `_`; then 7 to 15 digits. Digits are taken greedily: of a
longer digit run only the first 15 are used, the rest stay. The digits split into a
*middle* (all but the last 2, 5–13 digits) and the *last 2*. Replacement: the prefix kept
as is, one `*` per middle digit, then the last 2 digits. Fewer than 7 digits after the
prefix: no match. No boundary requirement.

**3. E-mail** (crude): the match begins at a character that is neither whitespace nor
`/`, and there must be a word boundary just before it. In practice that means a word
character preceded by a non-word character or text start, or a non-word character preceded
by a word character. From there, a run of non-whitespace, non-`/` characters must reach an
`@`. The run is greedy, so the **last** reachable `@` before any whitespace or `/` is
used. Then at least one non-whitespace character follows, and the match runs to the next
whitespace (or end of text), **including** any `/`, punctuation, `>` and so on. Candidate
starts are tried left to right; the first that succeeds wins. Replacement: the first
character, then `...@...`. Everything else up to the next whitespace disappears.

**4. Group id, v2**: an optional D-Bus path head, then 2 + 39 + 2 base64ish characters,
then one `=` or `_`. The path head is the literal text `/org/asamk/Signal/`, then any
characters except line terminators (greedy, as far right as still allows a match), then
the literal `Groups/`. With the head, the first 2 base64ish characters directly follow
`Groups/`. Replacement: the head (if any) and the first 2 characters kept, 39 `*`, the last
2 characters kept. The terminating `=`/`_` is **dropped**. No boundary requirement: within
a longer base64ish run, the earliest start that is followed by exactly 43 base64ish
characters and then `=`/`_` wins. Because `_` is both a base64ish character and a
terminator, underscore-heavy identifiers of 44+ characters can match.

**5. Group id, v1**: same as v2 but with 2 + 18 + 2 characters and a terminator of `==`
or `__`. Replacement: the head/first 2 kept, 18 `*`, the last 2 kept. The two-character
terminator is **dropped**.

**6. Domain name** (case-insensitive): one or more labels, each one or more ASCII
letters/digits followed by `.` (no hyphens in these labels), then a final label made of
ASCII letters, digits and `-` that contains at least one letter or `-`. Matching is greedy
and has no boundary requirement, so it can start in the middle of a word: it starts at the
first letter/digit after a `_`, `-`, space, etc. The final label extends as far as letters,
digits and `-` go. If no final label with a letter fits, the match backs off to fewer
labels, e.g. `foo.bar.123` matches as `foo.bar`. A match is censored only if both:
- the final label, lowercased, is one of these 100 TLDs: `com net org jp de uk fr br it ru
  es me gov pl ca au cn co in nl edu info eu ch id at kr cz mx be tv se tr tw al ua ir vn
  cl sk ly cc to no fi us pt dk ar hu tk gr il news ro my biz ie za nz sg ee th io xyz pe
  bg hk lt link ph club si site mobi by cat wiki la ga xxx cf hr ng jobs online kz ug gq
  ae is lv pro fm tips ms sa app`; and
- the matched text does not end with `whispersystems.org` and does not end with
  `signal.org`. This is a case-sensitive suffix test on the raw match, with no dot-boundary
  check: `notsignal.org` is also exempt, but `chat.Signal.org` is censored.

Replacement: `***.` followed by the final label as written (original case). Otherwise the
match is left unchanged.

Because the logger name is part of the text, the detailed-format name
`org.asamk.signal.App` becomes `***.App` (`app` is in the list). Observed:
`2026-10-29T09:22:54.695Z [main] DEBUG ***.App - Starting signal-cli 0.14.7`. No other
signal-cli logger ends in a listed TLD. In the simple format the name has no dot, so `App`
stays.

**7. IPv4**: word boundary, four decimal octets separated by `.`, word boundary. Each octet
is `250`–`255`, `200`–`249`, or one or two digits optionally preceded by `0`/`1`. Each
octet is therefore 0–255 and may carry leading zeros up to 3 characters (e.g. `010`,
`001`). The whole match is replaced by `...ipv4...`. Because a `.` is a non-word
character, `1.2.3.4.5` matches its first four octets. Port suffixes stay (`...ipv4...:8080`).

### 6.4 Worked examples

Each pair is input text → output after all seven rules:

```
Sending to +4915112345678 now
→ Sending to +***********78 now

+12345678 and +1234567 and +123456 and +1234567890123456789
→ +******78 and +*****67 and +123456 and +*************456789

file account_4915112345678.db %2B4915112345678 %2b4915112345678
→ file account_***********78.db %2B***********78 %2b4915112345678

uuid 2c2a7cd9-5a8b-4e1f-9a0c-3b4d5e6f7a8b done
→ uuid ********-****-****-****-**********8b done

UUID 2C2A7CD9_5A8B_4E1F_9A0C_3B4D5E6F7A8B JOB::2c2a7cd9-5a8b-4e1f-9a0c-3b4d5e6f7a8b
→ UUID ********-****-****-****-**********8B JOB::2c2a7cd9-5a8b-4e1f-9a0c-3b4d5e6f7a8b

Mail alice@example.com, contact /home/u/bob@mail.de x
→ Mail a...@... contact /home/u/b...@... x

<alice@example.com> and "x":"y@z"
→ <a...@... and "x...@...

group abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ= v1 abcdefghijklmnopqrstuv==
→ group ab***************************************PQ v1 ab******************uv

path /org/asamk/Signal/_4915112345678/Groups/abcdefghijklmnopqrstuv__
→ path /org/asamk/Signal/_***********78/Groups/ab******************uv

Connecting to chat.signal.org and storage.example.com:443 and cdn.whispersystems.org
→ Connecting to chat.signal.org and ***.com:443 and cdn.whispersystems.org

www.Example.CO.uk foo-bar.com my_site.de example.com-foo chat.Signal.org App org.asamk.signal.App notsignal.org
→ ***.uk foo-***.com my_***.de example.com-foo ***.org App ***.App notsignal.org

ip 192.168.1.20 and 10.0.0.1:8080 and 256.1.1.1 and 1.2.3.4.5
→ ip ...ipv4... and ...ipv4...:8080 and 256.1.1.1 and ...ipv4....5

2026-10-29T09:22:29.953Z [main] DEBUG org.asamk.signal.util.IOUtils - XDG_RUNTIME_DIR not set
→ unchanged
```

Notes on the examples:
- In the e-mail lines, the comma after `example.com` and the closing `>` are swallowed,
  because the match runs to the next whitespace. The e-mail rule runs before the domain
  rule, so a domain inside an address never becomes `***.de`.
- In the D-Bus path line, the phone rule (step 2) has already starred the `_`-prefixed
  number in the path before the group rule runs. Group ids that contain `+` followed by 7+
  digits get partly starred by step 2. The `*` then breaks the base64ish run, so the group
  rule may no longer match them.
- `example.com-foo`: the final label is `com-foo`, which is not a listed TLD, so it is
  unchanged.
- In the timestamp, `29.953Z` is a domain-shaped match (final label `953Z`), but `953z` is
  not a TLD, so it is unchanged. Dotted logger names and Java frame text
  (`java.lang.Thread.run`, `Thread.java`) are likewise unchanged unless the final label is
  a listed TLD.
- `256.1.1.1`: no octet window starts on a word boundary with valid octets, so it is
  unchanged.

Observed check: `-vvv --scrub-log version` output differs from `-vvv version` (ignoring
timestamps and pids) **only** in the `***.App` line. The JDK trace lines, `XDG_RUNTIME_DIR`
and thread names are untouched.
