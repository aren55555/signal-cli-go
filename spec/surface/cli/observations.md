# signal-cli 0.14.7 CLI: raw black-box observations (account-free)

## Sources

- Binary: signal-cli 0.14.7 release (`/work/third_party/signal-cli-0.14.7/bin/signal-cli`,
  Java 25), observed black-box. No upstream source text is reproduced here.
- Every invocation was run as
  `JAVA_OPTS="$JAVA_OPTS -Djava.io.tmpdir=<jvmtmp>" timeout 60 signal-cli --config <CONFIG> ARGS... </dev/null`
  from working directory `/tmp`, with stdout and stderr captured to separate files (no
  TTY, `COLUMNS` unset, `LANG` unset, so argparse wrap width = 75 and the JVM default
  charset is used). `<CONFIG>` is a fresh scratch data directory (one per batch).
  `<SCRATCH>` replaces scratch-file paths.
- The header of each entry shows the argv after `--config <CONFIG>`. Where environment
  variables or data-directory contents mattered, a `Setup:` line follows the header.
- Only account-free cases: help, version, parse errors, unknown commands/options, output
  mode checks, and account-selection errors with no or unregistered accounts. Nothing
  here contacted Signal servers.
- Timestamps in verbose log lines (`2026-10-23T...Z`) and pids are run-specific.
- One earlier exploratory run is deliberately **excluded** as a sandbox artifact:
  `-a +15555550123 -o json register` got past all local checks and tried to reach the
  server; the sandbox egress proxy refused it, giving exit 3 and
  `Failed to register: java.io.IOException: Unexpected response code for CONNECT: 403 (PushNetworkException)`.
  It only shows that network failures during register map to exit 3 with the
  `Failed to register: ` prefix.

Batches:

1. Global options, help, version, parse errors (batch A).
2. Account selection, output-mode support, configuration files (batch B).
3. Every subcommand with no arguments and no local account (batch C).
4. `CMD --help` for every subcommand (batch D).
5. Per-command parse-error and not-registered probes made while writing the command
   specs (batches G1-G4; command groups as in the per-command files).

## 1. Global (batch A)

### `signal-cli --config <CONFIG> `

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli [-h] [--version] [-v] [--log-file LOG_FILE] [--scrub-log]
                  [-d DATA_DIR] [-a ACCOUNT] [--bus-name GLOBAL-BUS-NAME]
                  [-o {plain-text,json}]
                  [--service-environment {live,staging}]
                  [--trust-new-identities {always,on-first-use,never}]
                  [--disable-send-log] [--dbus | --dbus-system]
                  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
                  ...
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli [-h] [--version] [-v] [--log-file LOG_FILE] [--scrub-log]
                  [-d DATA_DIR] [-a ACCOUNT] [--bus-name GLOBAL-BUS-NAME]
                  [-o {plain-text,json}]
                  [--service-environment {live,staging}]
                  [--trust-new-identities {always,on-first-use,never}]
                  [--disable-send-log] [--dbus | --dbus-system]
                  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
                  ...

Commandline interface for Signal.

named arguments:
  -h, --help             show this help message and exit
  --version              Show package version.
  -v, --verbose          Raise log  level  and  include  lib  signal  logs.
                         Specify  multiple  times   for   even  more  logs.
                         (default: 0)
  --log-file LOG_FILE    Write log output to  the  given file. If --verbose
                         is also given,  the  detailed  logs  will  only be
                         written to the log file.
  --scrub-log            Scrub  possibly  sensitive  information  from  the
                         log,  like  phone  numbers  and  UUIDs.  (default:
                         false)
  -d DATA_DIR, --data-dir DATA_DIR, -c DATA_DIR, --config DATA_DIR
                         Set  the  path  where   to  store  data  (Default:
                         $XDG_DATA_HOME/signal-cli        ,         $HOME/.
                         local/share/signal-cli).
  -a ACCOUNT, --account ACCOUNT, -u ACCOUNT, --username ACCOUNT
                         Specify your  phone  number,  that  will  be  your
                         identifier.
  --dbus                 Make request via user dbus. (default: false)
  --dbus-system          Make request via system dbus. (default: false)
  --bus-name GLOBAL-BUS-NAME
                         Specify  the  D-Bus  bus   name   to  connect  to.
                         (default: org.asamk.Signal)
  -o {plain-text,json}, --output {plain-text,json}
                         Choose to output in plain text or JSON
  --service-environment {live,staging}
                         Choose the server  environment  to  use. (default:
                         live)
  --trust-new-identities {always,on-first-use,never}
                         Choose when to trust new identities. (default: on-
                         first-use)
  --disable-send-log     Disable message send  log  (for resending messages
                         that recipient couldn't decrypt) (default: false)

subcommands:
  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
    acceptCall           Accept an incoming voice call.
    addDevice            Link another device  to  this  device. Only works,
                         if this is the primary device.
    addStickerPack       Install a sticker pack for this account.
    block                Block the given  contacts  or  groups (no messages
                         will be received)
    daemon               Run in daemon mode  and  provide  a JSON-RPC or an
                         experimental dbus interface.
    deleteLocalAccountData
                         Delete all  local  data  for  this  account.  Data
                         should  only  be   deleted   if   the  account  is
                         unregistered. CAUTION: This cannot be undone.
    finishChangeNumber   Verify the new number using  the code received via
                         SMS or voice.
    getAttachment        Retrieve an already  downloaded  attachment base64
                         encoded.
    getAvatar            Retrieve  the  avatar  of   a  contact,  contact's
                         profile or group base64 encoded.
    getSticker           Retrieve the  sticker  of  a  sticker  pack base64
                         encoded.
    getUserStatus        Check if the  specified  phone  number/s have been
                         registered
    hangupCall           Hang up an active voice call.
    joinGroup            Join a group via an invitation link.
    jsonRpc              Take  commands  from   standard   input  as  line-
                         delimited JSON RPC while receiving messages.
    link                 Link   to   an   existing   device,   instead   of
                         registering a new number.
    listAccounts         Show a list of registered accounts.
    listCalls            List active voice calls.
    listContacts         Show a  list  of  known  contacts  with  names and
                         profiles.
    listDevices          Show a list of linked devices.
    listGroups           List  group  information   including  names,  ids,
                         active status, blocked status and members
    listIdentities       List all  known  identity  keys  and  their  trust
                         status, fingerprint and safety number.
    listStickerPacks     Show a list of known sticker packs.
    quitGroup            Send a quit  group  message  to  all group members
                         and remove self from member list.
    receive              Query the server for new messages.
    register             Register  a  phone  number   with   SMS  or  voice
                         verification.
    rejectCall           Reject an incoming voice call.
    remoteDelete         Remotely delete a previously sent message.
    removeContact        Remove the details of a given contact
    removeDevice         Remove a linked device.
    removePin            Remove the registration lock pin.
    send                 Send a message to another user or group.
    sendAdminDelete      Send  admin  delete   message   for  a  previously
                         received or sent message.
    sendContacts         Send a  synchronization  message  with  the  local
                         contacts list to all linked devices.
    sendMessageRequestResponse
                         Send response  to  a  message  request  to  linked
                         devices.
    sendPaymentNotification
                         Send a payment notification.
    sendPinMessage       Send pin  message  for  a  previously  received or
                         sent message.
    sendPollCreate       Create a poll  and  send  it  to  another  user or
                         group.
    sendPollTerminate    Terminate a poll and  send  it  to another user or
                         group.
    sendPollVote         Vote on a poll  and  send  it  to  another user or
                         group.
    sendReaction         Send reaction to  a  previously  received  or sent
                         message.
    sendReceipt          Send a read  or  viewed  receipt  to  a previously
                         received message.
    sendStory            Post a story to your Story.
    sendSyncRequest      Send a synchronization request  message to primary
                         device (for group, contacts, ...).
    sendTyping           Send typing message to  trigger a typing indicator
                         for the recipient.  Indicator  will  be  shown for
                         15seconds unless a  typing  STOP  message  is sent
                         first.
    sendUnpinMessage     Send unpin message  for  a  previously received or
                         sent message.
    setPin               Set a registration  lock  pin,  to  prevent others
                         from registering this number.
    startCall            Start an outgoing voice call.
    startChangeNumber    Change account to a new  phone  number with SMS or
                         voice verification.
    submitRateLimitChallenge
                         Submit  a  captcha  challenge  to  lift  the  rate
                         limit. This command should  only be necessary when
                         sending fails with a proof required error.
    trust                Set the trust level of a given number.
    unblock              Unblock the  given  contacts  or  groups (messages
                         will be received again)
    unregister           Unregister the  current  device  from  the  signal
                         server.
    updateAccount        Update  the  account  attributes   on  the  signal
                         server.
    updateConfiguration  Update signal  configs  and  sync  them  to linked
                         devices.
    updateContact        Update the details of a given contact
    updateDevice         Update a linked device.
    updateGroup          Create or update a group.
    updateProfile        Set a name, about  and  avatar  image for the user
                         profile
    uploadStickerPack    Upload  a  new  sticker   pack,  consisting  of  a
                         manifest file and the stickers images.
    verify               Verify the number using the  code received via SMS
                         or voice.

The global arguments  are  shown  with  'signal-cli  -h'  and  need to come
before the subcommand, while the  subcommand-specific arguments (shown with
'signal-cli SUBCOMMAND -h') need to be given after the subcommand.
```

stderr:
```
```

### `signal-cli --config <CONFIG> -h`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli [-h] [--version] [-v] [--log-file LOG_FILE] [--scrub-log]
                  [-d DATA_DIR] [-a ACCOUNT] [--bus-name GLOBAL-BUS-NAME]
                  [-o {plain-text,json}]
                  [--service-environment {live,staging}]
                  [--trust-new-identities {always,on-first-use,never}]
                  [--disable-send-log] [--dbus | --dbus-system]
                  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
                  ...

Commandline interface for Signal.

named arguments:
  -h, --help             show this help message and exit
  --version              Show package version.
  -v, --verbose          Raise log  level  and  include  lib  signal  logs.
                         Specify  multiple  times   for   even  more  logs.
                         (default: 0)
  --log-file LOG_FILE    Write log output to  the  given file. If --verbose
                         is also given,  the  detailed  logs  will  only be
                         written to the log file.
  --scrub-log            Scrub  possibly  sensitive  information  from  the
                         log,  like  phone  numbers  and  UUIDs.  (default:
                         false)
  -d DATA_DIR, --data-dir DATA_DIR, -c DATA_DIR, --config DATA_DIR
                         Set  the  path  where   to  store  data  (Default:
                         $XDG_DATA_HOME/signal-cli        ,         $HOME/.
                         local/share/signal-cli).
  -a ACCOUNT, --account ACCOUNT, -u ACCOUNT, --username ACCOUNT
                         Specify your  phone  number,  that  will  be  your
                         identifier.
  --dbus                 Make request via user dbus. (default: false)
  --dbus-system          Make request via system dbus. (default: false)
  --bus-name GLOBAL-BUS-NAME
                         Specify  the  D-Bus  bus   name   to  connect  to.
                         (default: org.asamk.Signal)
  -o {plain-text,json}, --output {plain-text,json}
                         Choose to output in plain text or JSON
  --service-environment {live,staging}
                         Choose the server  environment  to  use. (default:
                         live)
  --trust-new-identities {always,on-first-use,never}
                         Choose when to trust new identities. (default: on-
                         first-use)
  --disable-send-log     Disable message send  log  (for resending messages
                         that recipient couldn't decrypt) (default: false)

subcommands:
  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
    acceptCall           Accept an incoming voice call.
    addDevice            Link another device  to  this  device. Only works,
                         if this is the primary device.
    addStickerPack       Install a sticker pack for this account.
    block                Block the given  contacts  or  groups (no messages
                         will be received)
    daemon               Run in daemon mode  and  provide  a JSON-RPC or an
                         experimental dbus interface.
    deleteLocalAccountData
                         Delete all  local  data  for  this  account.  Data
                         should  only  be   deleted   if   the  account  is
                         unregistered. CAUTION: This cannot be undone.
    finishChangeNumber   Verify the new number using  the code received via
                         SMS or voice.
    getAttachment        Retrieve an already  downloaded  attachment base64
                         encoded.
    getAvatar            Retrieve  the  avatar  of   a  contact,  contact's
                         profile or group base64 encoded.
    getSticker           Retrieve the  sticker  of  a  sticker  pack base64
                         encoded.
    getUserStatus        Check if the  specified  phone  number/s have been
                         registered
    hangupCall           Hang up an active voice call.
    joinGroup            Join a group via an invitation link.
    jsonRpc              Take  commands  from   standard   input  as  line-
                         delimited JSON RPC while receiving messages.
    link                 Link   to   an   existing   device,   instead   of
                         registering a new number.
    listAccounts         Show a list of registered accounts.
    listCalls            List active voice calls.
    listContacts         Show a  list  of  known  contacts  with  names and
                         profiles.
    listDevices          Show a list of linked devices.
    listGroups           List  group  information   including  names,  ids,
                         active status, blocked status and members
    listIdentities       List all  known  identity  keys  and  their  trust
                         status, fingerprint and safety number.
    listStickerPacks     Show a list of known sticker packs.
    quitGroup            Send a quit  group  message  to  all group members
                         and remove self from member list.
    receive              Query the server for new messages.
    register             Register  a  phone  number   with   SMS  or  voice
                         verification.
    rejectCall           Reject an incoming voice call.
    remoteDelete         Remotely delete a previously sent message.
    removeContact        Remove the details of a given contact
    removeDevice         Remove a linked device.
    removePin            Remove the registration lock pin.
    send                 Send a message to another user or group.
    sendAdminDelete      Send  admin  delete   message   for  a  previously
                         received or sent message.
    sendContacts         Send a  synchronization  message  with  the  local
                         contacts list to all linked devices.
    sendMessageRequestResponse
                         Send response  to  a  message  request  to  linked
                         devices.
    sendPaymentNotification
                         Send a payment notification.
    sendPinMessage       Send pin  message  for  a  previously  received or
                         sent message.
    sendPollCreate       Create a poll  and  send  it  to  another  user or
                         group.
    sendPollTerminate    Terminate a poll and  send  it  to another user or
                         group.
    sendPollVote         Vote on a poll  and  send  it  to  another user or
                         group.
    sendReaction         Send reaction to  a  previously  received  or sent
                         message.
    sendReceipt          Send a read  or  viewed  receipt  to  a previously
                         received message.
    sendStory            Post a story to your Story.
    sendSyncRequest      Send a synchronization request  message to primary
                         device (for group, contacts, ...).
    sendTyping           Send typing message to  trigger a typing indicator
                         for the recipient.  Indicator  will  be  shown for
                         15seconds unless a  typing  STOP  message  is sent
                         first.
    sendUnpinMessage     Send unpin message  for  a  previously received or
                         sent message.
    setPin               Set a registration  lock  pin,  to  prevent others
                         from registering this number.
    startCall            Start an outgoing voice call.
    startChangeNumber    Change account to a new  phone  number with SMS or
                         voice verification.
    submitRateLimitChallenge
                         Submit  a  captcha  challenge  to  lift  the  rate
                         limit. This command should  only be necessary when
                         sending fails with a proof required error.
    trust                Set the trust level of a given number.
    unblock              Unblock the  given  contacts  or  groups (messages
                         will be received again)
    unregister           Unregister the  current  device  from  the  signal
                         server.
    updateAccount        Update  the  account  attributes   on  the  signal
                         server.
    updateConfiguration  Update signal  configs  and  sync  them  to linked
                         devices.
    updateContact        Update the details of a given contact
    updateDevice         Update a linked device.
    updateGroup          Create or update a group.
    updateProfile        Set a name, about  and  avatar  image for the user
                         profile
    uploadStickerPack    Upload  a  new  sticker   pack,  consisting  of  a
                         manifest file and the stickers images.
    verify               Verify the number using the  code received via SMS
                         or voice.

The global arguments  are  shown  with  'signal-cli  -h'  and  need to come
before the subcommand, while the  subcommand-specific arguments (shown with
'signal-cli SUBCOMMAND -h') need to be given after the subcommand.
```

stderr:
```
```

### `signal-cli --config <CONFIG> --version`

exit code: 0

stdout:
```
signal-cli 0.14.7
```

stderr:
```
```

### `signal-cli --config <CONFIG> -v --version`

exit code: 0

stdout:
```
signal-cli 0.14.7
```

stderr:
```
2026-10-23T06:16:18.103Z [main] DEBUG org.asamk.signal.util.IOUtils - XDG_RUNTIME_DIR not set, falling back to temp dir
```

### `signal-cli --config <CONFIG> --version bogus`

exit code: 0

stdout:
```
signal-cli 0.14.7
```

stderr:
```
```

### `signal-cli --config <CONFIG> --vers`

exit code: 0

stdout:
```
signal-cli 0.14.7
```

stderr:
```
```

### `signal-cli --config <CONFIG> version`

exit code: 0

stdout:
```
signal-cli 0.14.7
```

stderr:
```
```

### `signal-cli --config <CONFIG> -o json version`

exit code: 0

stdout:
```
{"version":"0.14.7"}
```

stderr:
```
```

### `signal-cli --config <CONFIG> -o plain-text version`

exit code: 0

stdout:
```
signal-cli 0.14.7
```

stderr:
```
```

### `signal-cli --config <CONFIG> --output=json version`

exit code: 0

stdout:
```
{"version":"0.14.7"}
```

stderr:
```
```

### `signal-cli --config <CONFIG> -ojson version`

exit code: 0

stdout:
```
{"version":"0.14.7"}
```

stderr:
```
```

### `signal-cli --config <CONFIG> -oplain-text version`

exit code: 0

stdout:
```
signal-cli 0.14.7
```

stderr:
```
```

### `signal-cli --config <CONFIG> version extra`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli version [-h]
signal-cli: error: unrecognized arguments: 'extra'
```

### `signal-cli --config <CONFIG> version -h --bogus`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli version [-h]

named arguments:
  -h, --help             show this help message and exit
```

stderr:
```
```

### `signal-cli --config <CONFIG> -h bogus`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli [-h] [--version] [-v] [--log-file LOG_FILE] [--scrub-log]
                  [-d DATA_DIR] [-a ACCOUNT] [--bus-name GLOBAL-BUS-NAME]
                  [-o {plain-text,json}]
                  [--service-environment {live,staging}]
                  [--trust-new-identities {always,on-first-use,never}]
                  [--disable-send-log] [--dbus | --dbus-system]
                  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
                  ...

Commandline interface for Signal.

named arguments:
  -h, --help             show this help message and exit
  --version              Show package version.
  -v, --verbose          Raise log  level  and  include  lib  signal  logs.
                         Specify  multiple  times   for   even  more  logs.
                         (default: 0)
  --log-file LOG_FILE    Write log output to  the  given file. If --verbose
                         is also given,  the  detailed  logs  will  only be
                         written to the log file.
  --scrub-log            Scrub  possibly  sensitive  information  from  the
                         log,  like  phone  numbers  and  UUIDs.  (default:
                         false)
  -d DATA_DIR, --data-dir DATA_DIR, -c DATA_DIR, --config DATA_DIR
                         Set  the  path  where   to  store  data  (Default:
                         $XDG_DATA_HOME/signal-cli        ,         $HOME/.
                         local/share/signal-cli).
  -a ACCOUNT, --account ACCOUNT, -u ACCOUNT, --username ACCOUNT
                         Specify your  phone  number,  that  will  be  your
                         identifier.
  --dbus                 Make request via user dbus. (default: false)
  --dbus-system          Make request via system dbus. (default: false)
  --bus-name GLOBAL-BUS-NAME
                         Specify  the  D-Bus  bus   name   to  connect  to.
                         (default: org.asamk.Signal)
  -o {plain-text,json}, --output {plain-text,json}
                         Choose to output in plain text or JSON
  --service-environment {live,staging}
                         Choose the server  environment  to  use. (default:
                         live)
  --trust-new-identities {always,on-first-use,never}
                         Choose when to trust new identities. (default: on-
                         first-use)
  --disable-send-log     Disable message send  log  (for resending messages
                         that recipient couldn't decrypt) (default: false)

subcommands:
  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
    acceptCall           Accept an incoming voice call.
    addDevice            Link another device  to  this  device. Only works,
                         if this is the primary device.
    addStickerPack       Install a sticker pack for this account.
    block                Block the given  contacts  or  groups (no messages
                         will be received)
    daemon               Run in daemon mode  and  provide  a JSON-RPC or an
                         experimental dbus interface.
    deleteLocalAccountData
                         Delete all  local  data  for  this  account.  Data
                         should  only  be   deleted   if   the  account  is
                         unregistered. CAUTION: This cannot be undone.
    finishChangeNumber   Verify the new number using  the code received via
                         SMS or voice.
    getAttachment        Retrieve an already  downloaded  attachment base64
                         encoded.
    getAvatar            Retrieve  the  avatar  of   a  contact,  contact's
                         profile or group base64 encoded.
    getSticker           Retrieve the  sticker  of  a  sticker  pack base64
                         encoded.
    getUserStatus        Check if the  specified  phone  number/s have been
                         registered
    hangupCall           Hang up an active voice call.
    joinGroup            Join a group via an invitation link.
    jsonRpc              Take  commands  from   standard   input  as  line-
                         delimited JSON RPC while receiving messages.
    link                 Link   to   an   existing   device,   instead   of
                         registering a new number.
    listAccounts         Show a list of registered accounts.
    listCalls            List active voice calls.
    listContacts         Show a  list  of  known  contacts  with  names and
                         profiles.
    listDevices          Show a list of linked devices.
    listGroups           List  group  information   including  names,  ids,
                         active status, blocked status and members
    listIdentities       List all  known  identity  keys  and  their  trust
                         status, fingerprint and safety number.
    listStickerPacks     Show a list of known sticker packs.
    quitGroup            Send a quit  group  message  to  all group members
                         and remove self from member list.
    receive              Query the server for new messages.
    register             Register  a  phone  number   with   SMS  or  voice
                         verification.
    rejectCall           Reject an incoming voice call.
    remoteDelete         Remotely delete a previously sent message.
    removeContact        Remove the details of a given contact
    removeDevice         Remove a linked device.
    removePin            Remove the registration lock pin.
    send                 Send a message to another user or group.
    sendAdminDelete      Send  admin  delete   message   for  a  previously
                         received or sent message.
    sendContacts         Send a  synchronization  message  with  the  local
                         contacts list to all linked devices.
    sendMessageRequestResponse
                         Send response  to  a  message  request  to  linked
                         devices.
    sendPaymentNotification
                         Send a payment notification.
    sendPinMessage       Send pin  message  for  a  previously  received or
                         sent message.
    sendPollCreate       Create a poll  and  send  it  to  another  user or
                         group.
    sendPollTerminate    Terminate a poll and  send  it  to another user or
                         group.
    sendPollVote         Vote on a poll  and  send  it  to  another user or
                         group.
    sendReaction         Send reaction to  a  previously  received  or sent
                         message.
    sendReceipt          Send a read  or  viewed  receipt  to  a previously
                         received message.
    sendStory            Post a story to your Story.
    sendSyncRequest      Send a synchronization request  message to primary
                         device (for group, contacts, ...).
    sendTyping           Send typing message to  trigger a typing indicator
                         for the recipient.  Indicator  will  be  shown for
                         15seconds unless a  typing  STOP  message  is sent
                         first.
    sendUnpinMessage     Send unpin message  for  a  previously received or
                         sent message.
    setPin               Set a registration  lock  pin,  to  prevent others
                         from registering this number.
    startCall            Start an outgoing voice call.
    startChangeNumber    Change account to a new  phone  number with SMS or
                         voice verification.
    submitRateLimitChallenge
                         Submit  a  captcha  challenge  to  lift  the  rate
                         limit. This command should  only be necessary when
                         sending fails with a proof required error.
    trust                Set the trust level of a given number.
    unblock              Unblock the  given  contacts  or  groups (messages
                         will be received again)
    unregister           Unregister the  current  device  from  the  signal
                         server.
    updateAccount        Update  the  account  attributes   on  the  signal
                         server.
    updateConfiguration  Update signal  configs  and  sync  them  to linked
                         devices.
    updateContact        Update the details of a given contact
    updateDevice         Update a linked device.
    updateGroup          Create or update a group.
    updateProfile        Set a name, about  and  avatar  image for the user
                         profile
    uploadStickerPack    Upload  a  new  sticker   pack,  consisting  of  a
                         manifest file and the stickers images.
    verify               Verify the number using the  code received via SMS
                         or voice.

The global arguments  are  shown  with  'signal-cli  -h'  and  need to come
before the subcommand, while the  subcommand-specific arguments (shown with
'signal-cli SUBCOMMAND -h') need to be given after the subcommand.
```

stderr:
```
```

### `signal-cli --config <CONFIG> -h -o xml`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli [-h] [--version] [-v] [--log-file LOG_FILE] [--scrub-log]
                  [-d DATA_DIR] [-a ACCOUNT] [--bus-name GLOBAL-BUS-NAME]
                  [-o {plain-text,json}]
                  [--service-environment {live,staging}]
                  [--trust-new-identities {always,on-first-use,never}]
                  [--disable-send-log] [--dbus | --dbus-system]
                  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
                  ...

Commandline interface for Signal.

named arguments:
  -h, --help             show this help message and exit
  --version              Show package version.
  -v, --verbose          Raise log  level  and  include  lib  signal  logs.
                         Specify  multiple  times   for   even  more  logs.
                         (default: 0)
  --log-file LOG_FILE    Write log output to  the  given file. If --verbose
                         is also given,  the  detailed  logs  will  only be
                         written to the log file.
  --scrub-log            Scrub  possibly  sensitive  information  from  the
                         log,  like  phone  numbers  and  UUIDs.  (default:
                         false)
  -d DATA_DIR, --data-dir DATA_DIR, -c DATA_DIR, --config DATA_DIR
                         Set  the  path  where   to  store  data  (Default:
                         $XDG_DATA_HOME/signal-cli        ,         $HOME/.
                         local/share/signal-cli).
  -a ACCOUNT, --account ACCOUNT, -u ACCOUNT, --username ACCOUNT
                         Specify your  phone  number,  that  will  be  your
                         identifier.
  --dbus                 Make request via user dbus. (default: false)
  --dbus-system          Make request via system dbus. (default: false)
  --bus-name GLOBAL-BUS-NAME
                         Specify  the  D-Bus  bus   name   to  connect  to.
                         (default: org.asamk.Signal)
  -o {plain-text,json}, --output {plain-text,json}
                         Choose to output in plain text or JSON
  --service-environment {live,staging}
                         Choose the server  environment  to  use. (default:
                         live)
  --trust-new-identities {always,on-first-use,never}
                         Choose when to trust new identities. (default: on-
                         first-use)
  --disable-send-log     Disable message send  log  (for resending messages
                         that recipient couldn't decrypt) (default: false)

subcommands:
  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
    acceptCall           Accept an incoming voice call.
    addDevice            Link another device  to  this  device. Only works,
                         if this is the primary device.
    addStickerPack       Install a sticker pack for this account.
    block                Block the given  contacts  or  groups (no messages
                         will be received)
    daemon               Run in daemon mode  and  provide  a JSON-RPC or an
                         experimental dbus interface.
    deleteLocalAccountData
                         Delete all  local  data  for  this  account.  Data
                         should  only  be   deleted   if   the  account  is
                         unregistered. CAUTION: This cannot be undone.
    finishChangeNumber   Verify the new number using  the code received via
                         SMS or voice.
    getAttachment        Retrieve an already  downloaded  attachment base64
                         encoded.
    getAvatar            Retrieve  the  avatar  of   a  contact,  contact's
                         profile or group base64 encoded.
    getSticker           Retrieve the  sticker  of  a  sticker  pack base64
                         encoded.
    getUserStatus        Check if the  specified  phone  number/s have been
                         registered
    hangupCall           Hang up an active voice call.
    joinGroup            Join a group via an invitation link.
    jsonRpc              Take  commands  from   standard   input  as  line-
                         delimited JSON RPC while receiving messages.
    link                 Link   to   an   existing   device,   instead   of
                         registering a new number.
    listAccounts         Show a list of registered accounts.
    listCalls            List active voice calls.
    listContacts         Show a  list  of  known  contacts  with  names and
                         profiles.
    listDevices          Show a list of linked devices.
    listGroups           List  group  information   including  names,  ids,
                         active status, blocked status and members
    listIdentities       List all  known  identity  keys  and  their  trust
                         status, fingerprint and safety number.
    listStickerPacks     Show a list of known sticker packs.
    quitGroup            Send a quit  group  message  to  all group members
                         and remove self from member list.
    receive              Query the server for new messages.
    register             Register  a  phone  number   with   SMS  or  voice
                         verification.
    rejectCall           Reject an incoming voice call.
    remoteDelete         Remotely delete a previously sent message.
    removeContact        Remove the details of a given contact
    removeDevice         Remove a linked device.
    removePin            Remove the registration lock pin.
    send                 Send a message to another user or group.
    sendAdminDelete      Send  admin  delete   message   for  a  previously
                         received or sent message.
    sendContacts         Send a  synchronization  message  with  the  local
                         contacts list to all linked devices.
    sendMessageRequestResponse
                         Send response  to  a  message  request  to  linked
                         devices.
    sendPaymentNotification
                         Send a payment notification.
    sendPinMessage       Send pin  message  for  a  previously  received or
                         sent message.
    sendPollCreate       Create a poll  and  send  it  to  another  user or
                         group.
    sendPollTerminate    Terminate a poll and  send  it  to another user or
                         group.
    sendPollVote         Vote on a poll  and  send  it  to  another user or
                         group.
    sendReaction         Send reaction to  a  previously  received  or sent
                         message.
    sendReceipt          Send a read  or  viewed  receipt  to  a previously
                         received message.
    sendStory            Post a story to your Story.
    sendSyncRequest      Send a synchronization request  message to primary
                         device (for group, contacts, ...).
    sendTyping           Send typing message to  trigger a typing indicator
                         for the recipient.  Indicator  will  be  shown for
                         15seconds unless a  typing  STOP  message  is sent
                         first.
    sendUnpinMessage     Send unpin message  for  a  previously received or
                         sent message.
    setPin               Set a registration  lock  pin,  to  prevent others
                         from registering this number.
    startCall            Start an outgoing voice call.
    startChangeNumber    Change account to a new  phone  number with SMS or
                         voice verification.
    submitRateLimitChallenge
                         Submit  a  captcha  challenge  to  lift  the  rate
                         limit. This command should  only be necessary when
                         sending fails with a proof required error.
    trust                Set the trust level of a given number.
    unblock              Unblock the  given  contacts  or  groups (messages
                         will be received again)
    unregister           Unregister the  current  device  from  the  signal
                         server.
    updateAccount        Update  the  account  attributes   on  the  signal
                         server.
    updateConfiguration  Update signal  configs  and  sync  them  to linked
                         devices.
    updateContact        Update the details of a given contact
    updateDevice         Update a linked device.
    updateGroup          Create or update a group.
    updateProfile        Set a name, about  and  avatar  image for the user
                         profile
    uploadStickerPack    Upload  a  new  sticker   pack,  consisting  of  a
                         manifest file and the stickers images.
    verify               Verify the number using the  code received via SMS
                         or voice.

The global arguments  are  shown  with  'signal-cli  -h'  and  need to come
before the subcommand, while the  subcommand-specific arguments (shown with
'signal-cli SUBCOMMAND -h') need to be given after the subcommand.
```

stderr:
```
```

### `signal-cli --config <CONFIG> -o xml -h`

exit code: 1

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
```

stderr:
```
usage: signal-cli [-h] [--version] [-v] [--log-file LOG_FILE] [--scrub-log]
                  [-d DATA_DIR] [-a ACCOUNT] [--bus-name GLOBAL-BUS-NAME]
                  [-o {plain-text,json}]
                  [--service-environment {live,staging}]
                  [--trust-new-identities {always,on-first-use,never}]
                  [--disable-send-log] [--dbus | --dbus-system]
                  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
                  ...
signal-cli: error: argument -o/--output:  could  not  convert 'xml' (choose
from {plain-text,json})
```

### `signal-cli --config <CONFIG> bogus`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli [-h] [--version] [-v] [--log-file LOG_FILE] [--scrub-log]
                  [-d DATA_DIR] [-a ACCOUNT] [--bus-name GLOBAL-BUS-NAME]
                  [-o {plain-text,json}]
                  [--service-environment {live,staging}]
                  [--trust-new-identities {always,on-first-use,never}]
                  [--disable-send-log] [--dbus | --dbus-system]
                  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
                  ...
signal-cli: error:  invalid  choice:  'bogus'  (choose  from  'acceptCall',
'addDevice',         'addStickerPack',          'block',          'daemon',
'deleteLocalAccountData',      'finishChangeNumber',       'getAttachment',
'getAvatar',  'getSticker',  'getUserStatus',   'hangupCall',  'joinGroup',
'jsonRpc',    'link',    'listAccounts',    'listCalls',    'listContacts',
'listDevices',    'listGroups',    'listIdentities',    'listStickerPacks',
'quitGroup',   'receive',    'register',    'rejectCall',   'remoteDelete',
'removeContact', 'removeDevice',  'removePin',  'send',  'sendAdminDelete',
'sendContacts',  'sendMessageRequestResponse',   'sendPaymentNotification',
'sendPinMessage',  'sendPollCreate',  'sendPollTerminate',  'sendPollVote',
'sendReaction',     'sendReceipt',      'sendStory',     'sendSyncRequest',
'sendTyping',       'sendUnpinMessage',        'setPin',       'startCall',
'startChangeNumber',   'submitRateLimitChallenge',    'trust',   'unblock',
'unregister',  'updateAccount',   'updateConfiguration',   'updateContact',
'updateDevice',   'updateGroup',    'updateProfile',   'uploadStickerPack',
'verify', 'version')

Did you mean:
	trust
```

### `signal-cli --config <CONFIG> `

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli [-h] [--version] [-v] [--log-file LOG_FILE] [--scrub-log]
                  [-d DATA_DIR] [-a ACCOUNT] [--bus-name GLOBAL-BUS-NAME]
                  [-o {plain-text,json}]
                  [--service-environment {live,staging}]
                  [--trust-new-identities {always,on-first-use,never}]
                  [--disable-send-log] [--dbus | --dbus-system]
                  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
                  ...
signal-cli:  error:  invalid   choice:   ''   (choose   from  'acceptCall',
'addDevice',         'addStickerPack',          'block',          'daemon',
'deleteLocalAccountData',      'finishChangeNumber',       'getAttachment',
'getAvatar',  'getSticker',  'getUserStatus',   'hangupCall',  'joinGroup',
'jsonRpc',    'link',    'listAccounts',    'listCalls',    'listContacts',
'listDevices',    'listGroups',    'listIdentities',    'listStickerPacks',
'quitGroup',   'receive',    'register',    'rejectCall',   'remoteDelete',
'removeContact', 'removeDevice',  'removePin',  'send',  'sendAdminDelete',
'sendContacts',  'sendMessageRequestResponse',   'sendPaymentNotification',
'sendPinMessage',  'sendPollCreate',  'sendPollTerminate',  'sendPollVote',
'sendReaction',     'sendReceipt',      'sendStory',     'sendSyncRequest',
'sendTyping',       'sendUnpinMessage',        'setPin',       'startCall',
'startChangeNumber',   'submitRateLimitChallenge',    'trust',   'unblock',
'unregister',  'updateAccount',   'updateConfiguration',   'updateContact',
'updateDevice',   'updateGroup',    'updateProfile',   'uploadStickerPack',
'verify', 'version')

Did you mean:
	acceptCall
	addDevice
	addStickerPack
	block
	daemon
	deleteLocalAccountData
	finishChangeNumber
	getAttachment
	getAvatar
	getSticker
	getUserStatus
	hangupCall
	joinGroup
	jsonRpc
	link
	listAccounts
	listCalls
	listContacts
	listDevices
	listGroups
	listIdentities
	listStickerPacks
	quitGroup
	receive
	register
	rejectCall
	remoteDelete
	removeContact
	removeDevice
	removePin
	send
	sendAdminDelete
	sendContacts
	sendMessageRequestResponse
	sendPaymentNotification
	sendPinMessage
	sendPollCreate
	sendPollTerminate
	sendPollVote
	sendReaction
	sendReceipt
	sendStory
	sendSyncRequest
	sendTyping
	sendUnpinMessage
	setPin
	startCall
	startChangeNumber
	submitRateLimitChallenge
	trust
	unblock
	unregister
	updateAccount
	updateConfiguration
	updateContact
	updateDevice
	updateGroup
	updateProfile
	uploadStickerPack
	verify
	version
```

### `signal-cli --config <CONFIG> --bogus`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli [-h] [--version] [-v] [--log-file LOG_FILE] [--scrub-log]
                  [-d DATA_DIR] [-a ACCOUNT] [--bus-name GLOBAL-BUS-NAME]
                  [-o {plain-text,json}]
                  [--service-environment {live,staging}]
                  [--trust-new-identities {always,on-first-use,never}]
                  [--disable-send-log] [--dbus | --dbus-system]
                  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
                  ...
signal-cli: error: unrecognized arguments: '--bogus'

Did you mean:
	--dbus
```

### `signal-cli --config <CONFIG> -- version`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli [-h] [--version] [-v] [--log-file LOG_FILE] [--scrub-log]
                  [-d DATA_DIR] [-a ACCOUNT] [--bus-name GLOBAL-BUS-NAME]
                  [-o {plain-text,json}]
                  [--service-environment {live,staging}]
                  [--trust-new-identities {always,on-first-use,never}]
                  [--disable-send-log] [--dbus | --dbus-system]
                  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
                  ...
signal-cli: error: unrecognized arguments: 'version'
```

### `signal-cli --config <CONFIG> --d version`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli [-h] [--version] [-v] [--log-file LOG_FILE] [--scrub-log]
                  [-d DATA_DIR] [-a ACCOUNT] [--bus-name GLOBAL-BUS-NAME]
                  [-o {plain-text,json}]
                  [--service-environment {live,staging}]
                  [--trust-new-identities {always,on-first-use,never}]
                  [--disable-send-log] [--dbus | --dbus-system]
                  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
                  ...
signal-cli: error: ambiguous option: --d could match --data-dir, --dbus, --
dbus-system, --disable-send-log
```

### `signal-cli --config <CONFIG> -a`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli [-h] [--version] [-v] [--log-file LOG_FILE] [--scrub-log]
                  [-d DATA_DIR] [-a ACCOUNT] [--bus-name GLOBAL-BUS-NAME]
                  [-o {plain-text,json}]
                  [--service-environment {live,staging}]
                  [--trust-new-identities {always,on-first-use,never}]
                  [--disable-send-log] [--dbus | --dbus-system]
                  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
                  ...
signal-cli:  error:  argument   -a/--account/-u/--username:   expected  one
argument
```

### `signal-cli --config <CONFIG> -o`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli [-h] [--version] [-v] [--log-file LOG_FILE] [--scrub-log]
                  [-d DATA_DIR] [-a ACCOUNT] [--bus-name GLOBAL-BUS-NAME]
                  [-o {plain-text,json}]
                  [--service-environment {live,staging}]
                  [--trust-new-identities {always,on-first-use,never}]
                  [--disable-send-log] [--dbus | --dbus-system]
                  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
                  ...
signal-cli: error: argument -o/--output: expected one argument
```

### `signal-cli --config <CONFIG> -o xml version`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli [-h] [--version] [-v] [--log-file LOG_FILE] [--scrub-log]
                  [-d DATA_DIR] [-a ACCOUNT] [--bus-name GLOBAL-BUS-NAME]
                  [-o {plain-text,json}]
                  [--service-environment {live,staging}]
                  [--trust-new-identities {always,on-first-use,never}]
                  [--disable-send-log] [--dbus | --dbus-system]
                  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
                  ...
signal-cli: error: argument -o/--output:  could  not  convert 'xml' (choose
from {plain-text,json})
```

### `signal-cli --config <CONFIG> --service-environment prod version`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli [-h] [--version] [-v] [--log-file LOG_FILE] [--scrub-log]
                  [-d DATA_DIR] [-a ACCOUNT] [--bus-name GLOBAL-BUS-NAME]
                  [-o {plain-text,json}]
                  [--service-environment {live,staging}]
                  [--trust-new-identities {always,on-first-use,never}]
                  [--disable-send-log] [--dbus | --dbus-system]
                  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
                  ...
signal-cli:  error:  argument  --service-environment:   could  not  convert
'prod' (choose from {live,staging})
```

### `signal-cli --config <CONFIG> --trust-new-identities sometimes version`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli [-h] [--version] [-v] [--log-file LOG_FILE] [--scrub-log]
                  [-d DATA_DIR] [-a ACCOUNT] [--bus-name GLOBAL-BUS-NAME]
                  [-o {plain-text,json}]
                  [--service-environment {live,staging}]
                  [--trust-new-identities {always,on-first-use,never}]
                  [--disable-send-log] [--dbus | --dbus-system]
                  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
                  ...
signal-cli:  error:  argument  --trust-new-identities:  could  not  convert
'sometimes' (choose from {always,on-first-use,never})
```

### `signal-cli --config <CONFIG> --dbus --dbus-system version`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli [-h] [--version] [-v] [--log-file LOG_FILE] [--scrub-log]
                  [-d DATA_DIR] [-a ACCOUNT] [--bus-name GLOBAL-BUS-NAME]
                  [-o {plain-text,json}]
                  [--service-environment {live,staging}]
                  [--trust-new-identities {always,on-first-use,never}]
                  [--disable-send-log] [--dbus | --dbus-system]
                  {acceptCall,addDevice,addStickerPack,block,daemon,deleteLocalAccountData,finishChangeNumber,getAttachment,getAvatar,getSticker,getUserStatus,hangupCall,joinGroup,jsonRpc,link,listAccounts,listCalls,listContacts,listDevices,listGroups,listIdentities,listStickerPacks,quitGroup,receive,register,rejectCall,remoteDelete,removeContact,removeDevice,removePin,send,sendAdminDelete,sendContacts,sendMessageRequestResponse,sendPaymentNotification,sendPinMessage,sendPollCreate,sendPollTerminate,sendPollVote,sendReaction,sendReceipt,sendStory,sendSyncRequest,sendTyping,sendUnpinMessage,setPin,startCall,startChangeNumber,submitRateLimitChallenge,trust,unblock,unregister,updateAccount,updateConfiguration,updateContact,updateDevice,updateGroup,updateProfile,uploadStickerPack,verify,version}
                  ...
signal-cli: error: argument  --dbus-system:  not  allowed  with argument --
dbus
```

### `signal-cli --config <CONFIG> --log-file <SCRATCH>/lf.txt version`

exit code: 0

stdout:
```
signal-cli 0.14.7
```

stderr:
```
```

### `signal-cli --config <CONFIG> --scrub-log version`

exit code: 0

stdout:
```
signal-cli 0.14.7
```

stderr:
```
```

### `signal-cli --config <CONFIG> --service-environment staging version`

exit code: 0

stdout:
```
signal-cli 0.14.7
```

stderr:
```
```

### `signal-cli --config <CONFIG> --disable-send-log version`

exit code: 0

stdout:
```
signal-cli 0.14.7
```

stderr:
```
```

### `signal-cli --config <CONFIG> --trust-new-identities always version`

exit code: 0

stdout:
```
signal-cli 0.14.7
```

stderr:
```
```

### `signal-cli --config <CONFIG> --dbus version`

exit code: 2

stdout:
```
```

stderr:
```
Dbus client failed: Cannot Resolve Session Bus Address: DISPLAY variable not set
```

### `signal-cli --config <CONFIG> --dbus-system version`

exit code: 2

stdout:
```
```

stderr:
```
Dbus client failed: Cannot Resolve Session Bus Address: MachineId file can not be found
```

### `signal-cli --config <CONFIG> -a +15555550123 --dbus link`

exit code: 2

stdout:
```
```

stderr:
```
Dbus client failed: Cannot Resolve Session Bus Address: DISPLAY variable not set
```

### `signal-cli --config <CONFIG> -d /nonexistent/x version`

exit code: 3

stdout:
```
```

stderr:
```
Failed to read local accounts list
```

## 2. Account selection, output modes, config files (batch B)

### `signal-cli --config <CONFIG> listAccounts`

exit code: 0

stdout:
```
```

stderr:
```
```

### `signal-cli --config <CONFIG> -o json listAccounts`

exit code: 0

stdout:
```
[]
```

stderr:
```
```

### `signal-cli --config <CONFIG> send -m hi +15555550123`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> register`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> -a +15555550123 listContacts`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 --account +15555550124 listContacts`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550124 is not registered.
```

### `signal-cli --config <CONFIG> -a 3fa85f64-5717-4562-b3fc-2c963f66afa6 listContacts`

exit code: 1

stdout:
```
```

stderr:
```
User 3fa85f64-5717-4562-b3fc-2c963f66afa6 is not registered.
```

### `signal-cli --config <CONFIG> -a 12345 listContacts`

exit code: 1

stdout:
```
```

stderr:
```
Invalid account (phone number), make sure you include the country code.
```

### `signal-cli --config <CONFIG> -a 12345 register`

exit code: 1

stdout:
```
```

stderr:
```
Invalid account (phone number), make sure you include the country code.
```

### `signal-cli --config <CONFIG> -a 12345 verify 123456`

exit code: 1

stdout:
```
```

stderr:
```
Invalid account (phone number), make sure you include the country code.
```

### `signal-cli --config <CONFIG> -a +15555550123 link`

exit code: 1

stdout:
```
```

stderr:
```
You cannot specify a account (phone number) when linking
```

### `signal-cli --config <CONFIG> -a +15555550123 listAccounts`

exit code: 1

stdout:
```
```

stderr:
```
Command only works in multi-account mode
```

### `signal-cli --config <CONFIG> -a +15555550123 version`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 -o json version`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -o json link`

exit code: 1

stdout:
```
```

stderr:
```
Command doesn't support output type json
```

### `signal-cli --config <CONFIG> -o plain-text jsonRpc`

exit code: 1

stdout:
```
```

stderr:
```
Command doesn't support output type plain-text
```

### `signal-cli --config <CONFIG> listContacts`

Setup: Data dir <CONFIG> contains data/accounts.json listing two (non-existent, unregistered) LIVE accounts +15555550111 (path 111111) and +15555550122 (path 222222), version 2.

exit code: 1

stdout:
```
```

stderr:
```
Multiple users found, you need to specify an account (phone number) with -a
```

### `signal-cli --config <CONFIG> listAccounts`

Setup: Data dir <CONFIG> contains data/accounts.json listing two (non-existent, unregistered) LIVE accounts +15555550111 (path 111111) and +15555550122 (path 222222), version 2.

exit code: 0

stdout:
```
```

stderr:
```
WARN  MultiAccountManager - Ignoring +15555550111: User is not registered. (NotRegisteredException)
WARN  MultiAccountManager - Ignoring +15555550122: User is not registered. (NotRegisteredException)
```

### `signal-cli --config <CONFIG> -o json listAccounts`

Setup: Data dir <CONFIG> contains data/accounts.json listing two (non-existent, unregistered) LIVE accounts +15555550111 (path 111111) and +15555550122 (path 222222), version 2.

exit code: 0

stdout:
```
[]
```

stderr:
```
WARN  MultiAccountManager - Ignoring +15555550111: User is not registered. (NotRegisteredException)
WARN  MultiAccountManager - Ignoring +15555550122: User is not registered. (NotRegisteredException)
```

### `signal-cli --config <CONFIG> send -m x +15555550199`

Setup: Data dir <CONFIG> contains data/accounts.json listing two (non-existent, unregistered) LIVE accounts +15555550111 (path 111111) and +15555550122 (path 222222), version 2.

exit code: 1

stdout:
```
```

stderr:
```
Multiple users found, you need to specify an account (phone number) with -a
```

### `signal-cli --config <CONFIG> version`

Setup: Environment: SIGNAL_CLI_CONFIG=<SCRATCH>/out.json containing {"output":"json"}

exit code: 0

stdout:
```
{"version":"0.14.7"}
```

stderr:
```
```

### `signal-cli --config <CONFIG> version`

Setup: Environment: SIGNAL_CLI_CONFIG=<SCRATCH>/acct.json containing {"account":"+15555550123"}

exit code: 0

stdout:
```
signal-cli 0.14.7
```

stderr:
```
```

### `signal-cli --config <CONFIG> version`

Setup: Environment: SIGNAL_CLI_CONFIG=<SCRATCH>/bad.json containing the text {bad

exit code: 1

stdout:
```
```

stderr:
```
Failed to load config from <SCRATCH>/bad.json: Unexpected character ('b' (code 98)): was expecting double-quote to start field name
 at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: 1, column: 2]
```

### `signal-cli --config <CONFIG> version`

Setup: Environment: SIGNAL_CLI_CONFIG=<SCRATCH>/badval.json containing {"output":"xml"}

exit code: 1

stdout:
```
```

stderr:
```
Failed to parse configuration file(s): Cannot construct instance of `org.asamk.signal.OutputType`, problem: Invalid output type: xml
 at [Source: UNKNOWN; byte offset: #UNKNOWN] (through reference chain: org.asamk.signal.GlobalConfig["output"])
```

### `signal-cli --config <CONFIG> version`

Setup: Environment: SIGNAL_CLI_CONFIG=<SCRATCH>/unk.json containing {"unknownKey":1}

exit code: 1

stdout:
```
```

stderr:
```
Failed to parse configuration file(s): Unrecognized field "unknownKey" (class org.asamk.signal.GlobalConfig), not marked as ignorable (12 known properties: "logFile", "output", "verbose", "dataDir", "trustNewIdentities", "scrubLog", "busName", "dbus", "account", "serviceEnvironment", "disableSendLog", "dbusSystem")
 at [Source: UNKNOWN; byte offset: #UNKNOWN] (through reference chain: org.asamk.signal.GlobalConfig["unknownKey"])
```

### `signal-cli --config <CONFIG> version`

Setup: Environment: SIGNAL_CLI_CONFIG=<SCRATCH>/pt.json containing {"output":"PLAIN_TEXT"}

exit code: 0

stdout:
```
signal-cli 0.14.7
```

stderr:
```
```

### `signal-cli --config <CONFIG> version`

Setup: Environment: XDG_CONFIG_HOME=<SCRATCH>/xdg, with <SCRATCH>/xdg/signal-cli/config.json containing {"output":"json"}; SIGNAL_CLI_CONFIG unset

exit code: 0

stdout:
```
{"version":"0.14.7"}
```

stderr:
```
```

### `signal-cli --config <CONFIG> version`

Setup: Environment: XDG_CONFIG_HOME=<SCRATCH>/xdg (config {"output":"json"}) AND SIGNAL_CLI_CONFIG=<SCRATCH>/pt.json ({"output":"PLAIN_TEXT"}): shows the XDG file is NOT read when SIGNAL_CLI_CONFIG is set

exit code: 0

stdout:
```
signal-cli 0.14.7
```

stderr:
```
```

### `signal-cli --config <CONFIG> -o plain-text version`

Setup: Environment: XDG_CONFIG_HOME=<SCRATCH>/xdg (config {"output":"json"}); command-line -o overrides

exit code: 0

stdout:
```
signal-cli 0.14.7
```

stderr:
```
```

## 3. Subcommands with no arguments, empty data dir (batch C)

### `signal-cli --config <CONFIG> acceptCall`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli acceptCall [-h] --call-id CALL_ID
signal-cli: error: argument --call-id is required
```

### `signal-cli --config <CONFIG> addDevice`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli addDevice [-h] --uri URI
signal-cli: error: argument --uri is required
```

### `signal-cli --config <CONFIG> addStickerPack`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli addStickerPack [-h] --uri URI [URI ...]
signal-cli: error: argument --uri is required
```

### `signal-cli --config <CONFIG> block`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> deleteLocalAccountData`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> finishChangeNumber`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli finishChangeNumber [-h] -v VERIFICATION_CODE [-p PIN]
                  number
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> getAttachment`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli getAttachment [-h] --id ID (--recipient RECIPIENT |
                  -g GROUP_ID)
signal-cli: error: argument --id is required
```

### `signal-cli --config <CONFIG> getAvatar`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli getAvatar [-h] (-c CONTACT | -p PROFILE | -g GROUP_ID)
signal-cli: error: one  of  the  arguments  -c/--contact -p/--profile -g/--
group-id is required
```

### `signal-cli --config <CONFIG> getSticker`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli getSticker [-h] --pack-id PACK_ID --sticker-id STICKER_ID
signal-cli: error: argument --sticker-id is required
```

### `signal-cli --config <CONFIG> getUserStatus`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> hangupCall`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli hangupCall [-h] --call-id CALL_ID
signal-cli: error: argument --call-id is required
```

### `signal-cli --config <CONFIG> joinGroup`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli joinGroup [-h] --uri URI
signal-cli: error: argument --uri is required
```

### `signal-cli --config <CONFIG> listCalls`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> listContacts`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> listDevices`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> listGroups`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> listIdentities`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> listStickerPacks`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> quitGroup`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli quitGroup [-h] -g GROUP_ID [--delete]
                  [--admin [ADMIN [ADMIN ...]]]
signal-cli: error: argument -g/--group-id/--group is required
```

### `signal-cli --config <CONFIG> receive`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> register`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> rejectCall`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli rejectCall [-h] --call-id CALL_ID
signal-cli: error: argument --call-id is required
```

### `signal-cli --config <CONFIG> remoteDelete`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli remoteDelete [-h] -t TARGET_TIMESTAMP
                  [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [recipient [recipient ...]]
signal-cli: error: argument -t/--target-timestamp is required
```

### `signal-cli --config <CONFIG> removeContact`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli removeContact [-h] [--hide | --forget] recipient
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> removeDevice`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli removeDevice [-h] -d DEVICE_ID
signal-cli: error: argument -d/--device-id/--deviceId is required
```

### `signal-cli --config <CONFIG> removePin`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> send`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> sendAdminDelete`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendAdminDelete [-h] [-g GROUP_ID [GROUP_ID ...]]
                  [--notify-self] -a TARGET_AUTHOR -t TARGET_TIMESTAMP
                  [--story]
signal-cli: error: argument -t/--target-timestamp is required
```

### `signal-cli --config <CONFIG> sendContacts`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> sendMessageRequestResponse`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendMessageRequestResponse [-h]
                  [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] --type {accept,delete}
                  [recipient [recipient ...]]
signal-cli: error: argument --type is required
```

### `signal-cli --config <CONFIG> sendPaymentNotification`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPaymentNotification [-h] --receipt RECEIPT
                  [--note NOTE] recipient
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> sendPinMessage`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPinMessage [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [-d PIN_DURATION] -a TARGET_AUTHOR
                  -t TARGET_TIMESTAMP [--story] [recipient [recipient ...]]
signal-cli: error: argument -t/--target-timestamp is required
```

### `signal-cli --config <CONFIG> sendPollCreate`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPollCreate [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] -q QUESTION [--no-multi]
                  -o OPTION [OPTION ...] [recipient [recipient ...]]
signal-cli: error: argument -o/--option is required
```

### `signal-cli --config <CONFIG> sendPollTerminate`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPollTerminate [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] --poll-timestamp POLL_TIMESTAMP
                  [recipient [recipient ...]]
signal-cli: error: argument --poll-timestamp is required
```

### `signal-cli --config <CONFIG> sendPollVote`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPollVote [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [--poll-author POLL_AUTHOR]
                  --poll-timestamp POLL_TIMESTAMP
                  [-o [OPTION [OPTION ...]]] --vote-count VOTE_COUNT
                  [recipient [recipient ...]]
signal-cli: error: argument --vote-count is required
```

### `signal-cli --config <CONFIG> sendReaction`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendReaction [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] -e EMOJI -a TARGET_AUTHOR
                  -t TARGET_TIMESTAMP [-r] [--story]
                  [recipient [recipient ...]]
signal-cli: error: argument -t/--target-timestamp is required
```

### `signal-cli --config <CONFIG> sendReceipt`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendReceipt [-h]
                  -t TARGET_TIMESTAMP [TARGET_TIMESTAMP ...]
                  [--type {read,viewed}] recipient
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> sendStory`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendStory [-h] -a ATTACHMENT [--no-replies] [-g GROUP_ID]
signal-cli: error: argument -a/--attachment is required
```

### `signal-cli --config <CONFIG> sendSyncRequest`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> sendTyping`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> sendUnpinMessage`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendUnpinMessage [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] -a TARGET_AUTHOR -t TARGET_TIMESTAMP
                  [--story] [recipient [recipient ...]]
signal-cli: error: argument -t/--target-timestamp is required
```

### `signal-cli --config <CONFIG> setPin`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli setPin [-h] pin
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> startCall`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli startCall [-h] recipient
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> startChangeNumber`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli startChangeNumber [-h] [-v] [--captcha CAPTCHA] number
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> submitRateLimitChallenge`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli submitRateLimitChallenge [-h] --challenge CHALLENGE
                  --captcha CAPTCHA
signal-cli: error: argument --captcha is required
```

### `signal-cli --config <CONFIG> trust`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli trust [-h] [-a | -v VERIFIED_SAFETY_NUMBER] recipient
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> unblock`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> unregister`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> updateAccount`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> updateConfiguration`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> updateContact`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateContact [-h] [-n NAME] [--given-name GIVEN_NAME]
                  [--family-name FAMILY_NAME]
                  [--nick-given-name NICK_GIVEN_NAME]
                  [--nick-family-name NICK_FAMILY_NAME] [--note NOTE]
                  [-e EXPIRATION] recipient
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> updateDevice`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateDevice [-h] -d DEVICE_ID -n DEVICE_NAME
signal-cli: error: argument -n/--device-name is required
```

### `signal-cli --config <CONFIG> updateGroup`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> updateProfile`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> uploadStickerPack`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli uploadStickerPack [-h] path
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> verify`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli verify [-h] [-p PIN] verification_code
signal-cli: error: too few arguments
```

## 4. Subcommand --help (batch D)

### `signal-cli --config <CONFIG> acceptCall --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli acceptCall [-h] --call-id CALL_ID

named arguments:
  -h, --help             show this help message and exit
  --call-id CALL_ID      The call ID to accept.
```

stderr:
```
```

### `signal-cli --config <CONFIG> addDevice --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli addDevice [-h] --uri URI

named arguments:
  -h, --help             show this help message and exit
  --uri URI              Specify the uri contained in  the QR code shown by
                         the new device.
```

stderr:
```
```

### `signal-cli --config <CONFIG> addStickerPack --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli addStickerPack [-h] --uri URI [URI ...]

named arguments:
  -h, --help             show this help message and exit
  --uri URI [URI ...]    Specify the uri of the  sticker pack. (e.g. https:
                         //signal.
                         art/addstickers/#pack_id=XXX&pack_key=XXX)
```

stderr:
```
```

### `signal-cli --config <CONFIG> block --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli block [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [recipient [recipient ...]]

positional arguments:
  recipient              Contact number

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Group ID
```

stderr:
```
```

### `signal-cli --config <CONFIG> daemon --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli daemon [-h] [--dbus] [--dbus-system]
                  [--bus-name BUS_NAME] [--socket [SOCKET]] [--tcp [TCP]]
                  [--http [HTTP]] [--no-receive-stdout]
                  [--receive-mode {on-start,on-connection,manual}]
                  [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]

named arguments:
  -h, --help             show this help message and exit
  --dbus                 Expose a DBus interface on the user bus.
  --dbus-system, --system
                         Expose a DBus interface on the system bus.
  --bus-name BUS_NAME    Specify the D-Bus bus name to connect to.
  --socket [SOCKET]      Expose a  JSON-RPC  interface  on  a  UNIX  socket
                         (default $XDG_RUNTIME_DIR/signal-cli/socket).
  --tcp [TCP]            Expose  a  JSON-RPC  interface  on  a  TCP  socket
                         (default localhost:7583).
  --http [HTTP]          Expose  a  JSON-RPC  interface  as  http  endpoint
                         (default localhost:8080).
  --no-receive-stdout    Don?t print received messages to stdout.
  --receive-mode {on-start,on-connection,manual}
                         Specify when to start receiving messages.
  --ignore-attachments   Don?t download attachments of received messages.
  --ignore-stories       Don?t receive story messages from the server.
  --ignore-avatars       Don't download avatars of received messages.
  --ignore-stickers      Don't download sticker packs of received messages.
  --send-read-receipts   Send read receipts for  all incoming data messages
                         (in addition to the default delivery receipts)
```

stderr:
```
```

### `signal-cli --config <CONFIG> deleteLocalAccountData --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli deleteLocalAccountData [-h] [--ignore-registered]

named arguments:
  -h, --help             show this help message and exit
  --ignore-registered    Delete the account  data  even  though the account
                         is still registered on the Signal servers.
```

stderr:
```
```

### `signal-cli --config <CONFIG> finishChangeNumber --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli finishChangeNumber [-h] -v VERIFICATION_CODE [-p PIN]
                  number

positional arguments:
  number                 The new phone number in E164 format.

named arguments:
  -h, --help             show this help message and exit
  -v VERIFICATION_CODE, --verification-code VERIFICATION_CODE
                         The verification  code  you  received  via  sms or
                         voice call.
  -p PIN, --pin PIN      The registration lock  PIN,  that  was  set by the
                         user (Optional)
```

stderr:
```
```

### `signal-cli --config <CONFIG> getAttachment --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli getAttachment [-h] --id ID (--recipient RECIPIENT |
                  -g GROUP_ID)

named arguments:
  -h, --help             show this help message and exit
  --id ID                The ID of the attachment file.
  --recipient RECIPIENT  Sender of the attachment
  -g GROUP_ID, --group-id GROUP_ID
                         Group in which the attachment was received
```

stderr:
```
```

### `signal-cli --config <CONFIG> getAvatar --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli getAvatar [-h] (-c CONTACT | -p PROFILE | -g GROUP_ID)

named arguments:
  -h, --help             show this help message and exit
  -c CONTACT, --contact CONTACT
                         Get a contact avatar
  -p PROFILE, --profile PROFILE
                         Get a profile avatar
  -g GROUP_ID, --group-id GROUP_ID
                         Get a group avatar
```

stderr:
```
```

### `signal-cli --config <CONFIG> getSticker --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli getSticker [-h] --pack-id PACK_ID --sticker-id STICKER_ID

named arguments:
  -h, --help             show this help message and exit
  --pack-id PACK_ID      The ID of the sticker pack.
  --sticker-id STICKER_ID
                         The ID of the sticker.
```

stderr:
```
```

### `signal-cli --config <CONFIG> getUserStatus --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli getUserStatus [-h] [--username [USERNAME [USERNAME ...]]]
                  [recipient [recipient ...]]

positional arguments:
  recipient              Phone number

named arguments:
  -h, --help             show this help message and exit
  --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
```

stderr:
```
```

### `signal-cli --config <CONFIG> hangupCall --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli hangupCall [-h] --call-id CALL_ID

named arguments:
  -h, --help             show this help message and exit
  --call-id CALL_ID      The call ID to hang up.
```

stderr:
```
```

### `signal-cli --config <CONFIG> joinGroup --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli joinGroup [-h] --uri URI

named arguments:
  -h, --help             show this help message and exit
  --uri URI              Specify the uri with the group invitation link.
```

stderr:
```
```

### `signal-cli --config <CONFIG> jsonRpc --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli jsonRpc [-h] [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]
                  [--receive-mode {on-start,on-connection,manual}]

named arguments:
  -h, --help             show this help message and exit
  --ignore-attachments   Don?t download attachments of received messages.
  --ignore-stories       Don?t receive story messages from the server.
  --ignore-avatars       Don't download avatars of received messages.
  --ignore-stickers      Don't download sticker packs of received messages.
  --send-read-receipts   Send read receipts for  all incoming data messages
                         (in addition to the default delivery receipts)
  --receive-mode {on-start,on-connection,manual}
                         Specify when to start receiving messages.
```

stderr:
```
```

### `signal-cli --config <CONFIG> link --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli link [-h] [-n NAME]

named arguments:
  -h, --help             show this help message and exit
  -n NAME, --name NAME   Specify a name to describe this new device.
```

stderr:
```
```

### `signal-cli --config <CONFIG> listAccounts --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli listAccounts [-h]

named arguments:
  -h, --help             show this help message and exit
```

stderr:
```
```

### `signal-cli --config <CONFIG> listCalls --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli listCalls [-h]

named arguments:
  -h, --help             show this help message and exit
```

stderr:
```
```

### `signal-cli --config <CONFIG> listContacts --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli listContacts [-h] [-a] [--blocked {true,false}]
                  [--name NAME] [--detailed] [--internal]
                  [recipient [recipient ...]]

positional arguments:
  recipient              Specify one ore more phone numbers to show.

named arguments:
  -h, --help             show this help message and exit
  -a, --all-recipients   Include all known recipients, not only contacts.
  --blocked {true,false}
                         Specify if  only  blocked  or  unblocked  contacts
                         should be shown (default: all contacts)
  --name NAME            Find contacts with  the  given  contact or profile
                         name.
  --detailed             List  the   contacts   with   more   details.   If
                         output=json, then this is always set
  --internal             Include internal information  that's  normally not
                         user visible
```

stderr:
```
```

### `signal-cli --config <CONFIG> listDevices --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli listDevices [-h]

named arguments:
  -h, --help             show this help message and exit
```

stderr:
```
```

### `signal-cli --config <CONFIG> listGroups --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli listGroups [-h] [-d] [-g [GROUP_ID [GROUP_ID ...]]]

named arguments:
  -h, --help             show this help message and exit
  -d, --detailed         List the members and  group  invite  links of each
                         group. If output=json, then this is always set
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]]
                         Specify one or more group IDs to show.
```

stderr:
```
```

### `signal-cli --config <CONFIG> listIdentities --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli listIdentities [-h] [-n NUMBER]

named arguments:
  -h, --help             show this help message and exit
  -n NUMBER, --number NUMBER
                         Only  show  identity  keys  for  the  given  phone
                         number.
```

stderr:
```
```

### `signal-cli --config <CONFIG> listStickerPacks --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli listStickerPacks [-h]

named arguments:
  -h, --help             show this help message and exit
```

stderr:
```
```

### `signal-cli --config <CONFIG> quitGroup --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli quitGroup [-h] -g GROUP_ID [--delete]
                  [--admin [ADMIN [ADMIN ...]]]

named arguments:
  -h, --help             show this help message and exit
  -g GROUP_ID, --group-id GROUP_ID, --group GROUP_ID
                         Specify the recipient group ID.
  --delete               Delete local group data  completely after quitting
                         group.
  --admin [ADMIN [ADMIN ...]]
                         Specify one  or  more  members  to  make  a  group
                         admin,  required  if  you're  currently  the  only
                         admin.
```

stderr:
```
```

### `signal-cli --config <CONFIG> receive --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli receive [-h] [-t TIMEOUT] [--max-messages MAX_MESSAGES]
                  [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]

named arguments:
  -h, --help             show this help message and exit
  -t TIMEOUT, --timeout TIMEOUT
                         Number  of  seconds  to   wait  for  new  messages
                         (negative values disable timeout)
  --max-messages MAX_MESSAGES
                         Maximum number  of  messages  to  receive,  before
                         returning.
  --ignore-attachments   Don?t download attachments of received messages.
  --ignore-stories       Don?t receive story messages from the server.
  --ignore-avatars       Don't download avatars of received messages.
  --ignore-stickers      Don't download sticker packs of received messages.
  --send-read-receipts   Send read receipts for  all incoming data messages
                         (in addition to the default delivery receipts)
```

stderr:
```
```

### `signal-cli --config <CONFIG> register --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli register [-h] [-v] [--captcha CAPTCHA] [--reregister]

named arguments:
  -h, --help             show this help message and exit
  -v, --voice            The verification should  be  done  over voice, not
                         SMS.
  --captcha CAPTCHA      The  captcha  token,   required   if  registration
                         failed with a captcha required error.
  --reregister           Register even if account is already registered
```

stderr:
```
```

### `signal-cli --config <CONFIG> rejectCall --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli rejectCall [-h] --call-id CALL_ID

named arguments:
  -h, --help             show this help message and exit
  --call-id CALL_ID      The call ID to reject.
```

stderr:
```
```

### `signal-cli --config <CONFIG> remoteDelete --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli remoteDelete [-h] -t TARGET_TIMESTAMP
                  [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -t TARGET_TIMESTAMP, --target-timestamp TARGET_TIMESTAMP
                         Specify the timestamp of the message to delete.
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -u [USERNAME [USERNAME ...]], --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
  --note-to-self
```

stderr:
```
```

### `signal-cli --config <CONFIG> removeContact --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli removeContact [-h] [--hide | --forget] recipient

positional arguments:
  recipient              Contact number

named arguments:
  -h, --help             show this help message and exit
  --hide                 Hide the contact  in  the  contact  list, but keep
                         the data.
  --forget               Delete all  data  associated  with  this  contact,
                         including identity keys and sessions.
```

stderr:
```
```

### `signal-cli --config <CONFIG> removeDevice --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli removeDevice [-h] -d DEVICE_ID

named arguments:
  -h, --help             show this help message and exit
  -d DEVICE_ID, --device-id DEVICE_ID, --deviceId DEVICE_ID
                         Specify  the  device  you   want  to  remove.  Use
                         listDevices to see the deviceIds.
```

stderr:
```
```

### `signal-cli --config <CONFIG> removePin --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli removePin [-h]

named arguments:
  -h, --help             show this help message and exit
```

stderr:
```
```

### `signal-cli --config <CONFIG> send --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli send [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [-a [ATTACHMENT [ATTACHMENT ...]]]
                  [--view-once] [-e] [--mention [MENTION [MENTION ...]]]
                  [--text-style [TEXT_STYLE [TEXT_STYLE ...]]]
                  [--quote-timestamp QUOTE_TIMESTAMP]
                  [--quote-author QUOTE_AUTHOR]
                  [--quote-message QUOTE_MESSAGE]
                  [--quote-mention [QUOTE_MENTION [QUOTE_MENTION ...]]]
                  [--quote-attachment [QUOTE_ATTACHMENT [QUOTE_ATTACHMENT ...]]]
                  [--quote-text-style [QUOTE_TEXT_STYLE [QUOTE_TEXT_STYLE ...]]]
                  [--sticker STICKER] [--preview-url PREVIEW_URL]
                  [--preview-title PREVIEW_TITLE]
                  [--preview-description PREVIEW_DESCRIPTION]
                  [--preview-image PREVIEW_IMAGE]
                  [--story-timestamp STORY_TIMESTAMP]
                  [--story-author STORY_AUTHOR]
                  [--edit-timestamp EDIT_TIMESTAMP] [--no-urgent]
                  [--voice-note] [-m MESSAGE | --message-from-stdin]
                  [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -u [USERNAME [USERNAME ...]], --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
  --note-to-self         Send the message to self.
  --notify-self          If  self  is  part  of  recipients/groups  send  a
                         normal message, not a sync message.
  -m MESSAGE, --message MESSAGE
                         Specify the message to be sent.
  --message-from-stdin   Read the message from standard input.
  -a [ATTACHMENT [ATTACHMENT ...]], --attachment [ATTACHMENT [ATTACHMENT ...]]
                         Add an attachment. Can be either  a file path or a
                         data  URI.  Data  URI   encoded  attachments  must
                         follow the RFC 2397. Additionally  a file name can
                         be       added,       e.g.       data:<MIME-TYPE>;
                         filename=<FILENAME>;base64,<BASE64 ENCODED DATA>.
  --view-once            Send the message as a view once message
  -e, --end-session, --endsession
                         Clear session state and send end session message.
  --mention [MENTION [MENTION ...]]
                         Mention  another  group   member  (syntax:  start:
                         length:recipientNumber). Unit of  start and length
                         is UTF-16 code units, NOT Unicode code points.
  --text-style [TEXT_STYLE [TEXT_STYLE ...]]
                         Style parts of  the  message  text (syntax: start:
                         length:STYLE). Unit of start  and length is UTF-16
                         code units, NOT Unicode code points.
  --quote-timestamp QUOTE_TIMESTAMP
                         Specify the timestamp of  a  previous message with
                         the recipient or group to  add  a quote to the new
                         message.
  --quote-author QUOTE_AUTHOR
                         Specify the number of  the  author of the original
                         message.
  --quote-message QUOTE_MESSAGE
                         Specify the message of the original message.
  --quote-mention [QUOTE_MENTION [QUOTE_MENTION ...]]
                         Quote  with  mention   of   another  group  member
                         (syntax: start:length:recipientNumber)
  --quote-attachment [QUOTE_ATTACHMENT [QUOTE_ATTACHMENT ...]]
                         Specify the attachments  of  the  original message
                         (syntax: contentType[:filename[:previewFile]]), e.
                         g.     'audio/aac'     or     'image/png:test.png:
                         /tmp/preview.jpg'.
  --quote-text-style [QUOTE_TEXT_STYLE [QUOTE_TEXT_STYLE ...]]
                         Quote  with  style  parts   of  the  message  text
                         (syntax: start:length:STYLE)
  --sticker STICKER      Send a sticker (syntax: stickerPackId:stickerId)
  --preview-url PREVIEW_URL
                         Specify the url  for  the  link  preview (the same
                         url must also appear in the message body).
  --preview-title PREVIEW_TITLE
                         Specify   the   title   for   the   link   preview
                         (mandatory).
  --preview-description PREVIEW_DESCRIPTION
                         Specify  the  description  for  the  link  preview
                         (optional).
  --preview-image PREVIEW_IMAGE
                         Specify  the  image  file  for  the  link  preview
                         (optional).
  --story-timestamp STORY_TIMESTAMP
                         Specify the timestamp of a story to reply to.
  --story-author STORY_AUTHOR
                         Specify the number of the author of the story.
  --edit-timestamp EDIT_TIMESTAMP
                         Specify the timestamp of  a  previous message with
                         the recipient or group to send an edited message.
  --no-urgent            Send the message without  the  urgent  flag, so no
                         push notification is triggered  for the recipient.
                         The message will still  be  delivered in real-time
                         if the recipient's app is active.
  --voice-note           Mark  audio  attachments  as  voice  notes.  Voice
                         notes are displayed inline in Signal clients.
```

stderr:
```
```

### `signal-cli --config <CONFIG> sendAdminDelete --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendAdminDelete [-h] [-g GROUP_ID [GROUP_ID ...]]
                  [--notify-self] -a TARGET_AUTHOR -t TARGET_TIMESTAMP
                  [--story]

named arguments:
  -h, --help             show this help message and exit
  -g GROUP_ID [GROUP_ID ...], --group-id GROUP_ID [GROUP_ID ...], --group GROUP_ID [GROUP_ID ...]
                         Specify the recipient group ID.
  --notify-self          If  self  is  part  of  recipients/groups  send  a
                         normal message, not a sync message.
  -a TARGET_AUTHOR, --target-author TARGET_AUTHOR
                         Specify the number of  the  author  of the message
                         to admin delete.
  -t TARGET_TIMESTAMP, --target-timestamp TARGET_TIMESTAMP
                         Specify the  timestamp  of  the  message  to admin
                         delete.
  --story                Admin delete a story instead of a normal message
```

stderr:
```
```

### `signal-cli --config <CONFIG> sendContacts --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendContacts [-h]

named arguments:
  -h, --help             show this help message and exit
```

stderr:
```
```

### `signal-cli --config <CONFIG> sendMessageRequestResponse --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendMessageRequestResponse [-h]
                  [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] --type {accept,delete}
                  [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -u [USERNAME [USERNAME ...]], --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
  --type {accept,delete}
                         Type of message request response
```

stderr:
```
```

### `signal-cli --config <CONFIG> sendPaymentNotification --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendPaymentNotification [-h] --receipt RECEIPT
                  [--note NOTE] recipient

positional arguments:
  recipient              Specify the recipient's phone number.

named arguments:
  -h, --help             show this help message and exit
  --receipt RECEIPT      The base64 encoded receipt blob.
  --note NOTE            Specify a note for the payment notification.
```

stderr:
```
```

### `signal-cli --config <CONFIG> sendPinMessage --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendPinMessage [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [-d PIN_DURATION] -a TARGET_AUTHOR
                  -t TARGET_TIMESTAMP [--story] [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -u [USERNAME [USERNAME ...]], --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
  --note-to-self         Send the pin message to self.
  --notify-self          If  self  is  part  of  recipients/groups  send  a
                         normal message, not a sync message.
  -d PIN_DURATION, --pin-duration PIN_DURATION
                         Specify the pin duration  in  seconds.  Use -1 for
                         forever.
  -a TARGET_AUTHOR, --target-author TARGET_AUTHOR
                         Specify the number of  the  author  of the message
                         to pin.
  -t TARGET_TIMESTAMP, --target-timestamp TARGET_TIMESTAMP
                         Specify the timestamp of the message to pin.
  --story                Pin a story instead of a normal message
```

stderr:
```
```

### `signal-cli --config <CONFIG> sendPollCreate --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendPollCreate [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] -q QUESTION [--no-multi]
                  -o OPTION [OPTION ...] [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -u [USERNAME [USERNAME ...]], --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
  --note-to-self         Send the message to self
  --notify-self          If  self  is  part  of  recipients/groups  send  a
                         normal message, not a sync message.
  -q QUESTION, --question QUESTION
                         Specify the poll question.
  --no-multi             Allow only  one  option  to  be  selected  by each
                         recipient.
  -o OPTION [OPTION ...], --option OPTION [OPTION ...]
                         The options for the poll
```

stderr:
```
```

### `signal-cli --config <CONFIG> sendPollTerminate --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendPollTerminate [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] --poll-timestamp POLL_TIMESTAMP
                  [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -u [USERNAME [USERNAME ...]], --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
  --note-to-self         Send the message to self
  --notify-self          If  self  is  part  of  recipients/groups  send  a
                         normal message, not a sync message.
  --poll-timestamp POLL_TIMESTAMP
                         Specify  the  timestamp   of   the  original  poll
                         message.
```

stderr:
```
```

### `signal-cli --config <CONFIG> sendPollVote --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendPollVote [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [--poll-author POLL_AUTHOR]
                  --poll-timestamp POLL_TIMESTAMP
                  [-o [OPTION [OPTION ...]]] --vote-count VOTE_COUNT
                  [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -u [USERNAME [USERNAME ...]], --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
  --note-to-self         Send the message to self
  --notify-self          If  self  is  part  of  recipients/groups  send  a
                         normal message, not a sync message.
  --poll-author POLL_AUTHOR
                         Specify the  number  of  the  author  of  the poll
                         message.
  --poll-timestamp POLL_TIMESTAMP
                         Specify  the  timestamp   of   the  original  poll
                         message.
  -o [OPTION [OPTION ...]], --option [OPTION [OPTION ...]]
                         The option indexes of the poll to vote for
  --vote-count VOTE_COUNT
                         Specify the number of  this  vote (increase by one
                         for every time you vote).
```

stderr:
```
```

### `signal-cli --config <CONFIG> sendReaction --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendReaction [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] -e EMOJI -a TARGET_AUTHOR
                  -t TARGET_TIMESTAMP [-r] [--story]
                  [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -u [USERNAME [USERNAME ...]], --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
  --note-to-self         Send the reaction to self.
  --notify-self          If  self  is  part  of  recipients/groups  send  a
                         normal message, not a sync message.
  -e EMOJI, --emoji EMOJI
                         Specify the  emoji,  should  be  a  single unicode
                         grapheme cluster.
  -a TARGET_AUTHOR, --target-author TARGET_AUTHOR
                         Specify the number of  the  author  of the message
                         to which to react.
  -t TARGET_TIMESTAMP, --target-timestamp TARGET_TIMESTAMP
                         Specify the timestamp of  the  message to which to
                         react.
  -r, --remove           Remove a reaction.
  --story                React to a story instead of a normal message
```

stderr:
```
```

### `signal-cli --config <CONFIG> sendReceipt --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendReceipt [-h]
                  -t TARGET_TIMESTAMP [TARGET_TIMESTAMP ...]
                  [--type {read,viewed}] recipient

positional arguments:
  recipient              Specify the sender's phone number.

named arguments:
  -h, --help             show this help message and exit
  -t TARGET_TIMESTAMP [TARGET_TIMESTAMP ...], --target-timestamp TARGET_TIMESTAMP [TARGET_TIMESTAMP ...]
                         Specify the timestamp of the  messages for which a
                         receipt should be sent.
  --type {read,viewed}   Specify  the  receipt   type   (default   is  read
                         receipt).
```

stderr:
```
```

### `signal-cli --config <CONFIG> sendStory --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendStory [-h] -a ATTACHMENT [--no-replies] [-g GROUP_ID]

named arguments:
  -h, --help             show this help message and exit
  -a ATTACHMENT, --attachment ATTACHMENT
                         Specify the file path  to  the  image  or video to
                         post as a story.
  --no-replies           Disable replies on this story.
  -g GROUP_ID, --group-id GROUP_ID
                         Specify a group  to  post  the  story  to. Without
                         this, posts to My Story.
```

stderr:
```
```

### `signal-cli --config <CONFIG> sendSyncRequest --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendSyncRequest [-h]

named arguments:
  -h, --help             show this help message and exit
```

stderr:
```
```

### `signal-cli --config <CONFIG> sendTyping --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendTyping [-h] [-g [GROUP_ID [GROUP_ID ...]]] [-s]
                  [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -s, --stop             Send a typing STOP message.
```

stderr:
```
```

### `signal-cli --config <CONFIG> sendUnpinMessage --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli sendUnpinMessage [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] -a TARGET_AUTHOR -t TARGET_TIMESTAMP
                  [--story] [recipient [recipient ...]]

positional arguments:
  recipient              Specify the recipients' phone number.

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Specify the recipient group ID.
  -u [USERNAME [USERNAME ...]], --username [USERNAME [USERNAME ...]]
                         Specify the recipient username or username link.
  --note-to-self         Send the unpin message to self.
  --notify-self          If  self  is  part  of  recipients/groups  send  a
                         normal message, not a sync message.
  -a TARGET_AUTHOR, --target-author TARGET_AUTHOR
                         Specify the number of  the  author  of the message
                         to unpin.
  -t TARGET_TIMESTAMP, --target-timestamp TARGET_TIMESTAMP
                         Specify the timestamp of the message to unpin.
  --story                Unpin a story instead of a normal message
```

stderr:
```
```

### `signal-cli --config <CONFIG> setPin --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli setPin [-h] pin

positional arguments:
  pin                    The registration lock PIN,  that  will be required
                         for new  registrations  (resets  after  7  days of
                         inactivity)

named arguments:
  -h, --help             show this help message and exit
```

stderr:
```
```

### `signal-cli --config <CONFIG> startCall --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli startCall [-h] recipient

positional arguments:
  recipient              Specify the recipient's phone number or UUID.

named arguments:
  -h, --help             show this help message and exit
```

stderr:
```
```

### `signal-cli --config <CONFIG> startChangeNumber --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli startChangeNumber [-h] [-v] [--captcha CAPTCHA] number

positional arguments:
  number                 The new phone number in E164 format.

named arguments:
  -h, --help             show this help message and exit
  -v, --voice            The verification should  be  done  over voice, not
                         SMS.
  --captcha CAPTCHA      The  captcha  token,  required  if  change  number
                         failed with a captcha required error.
```

stderr:
```
```

### `signal-cli --config <CONFIG> submitRateLimitChallenge --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli submitRateLimitChallenge [-h] --challenge CHALLENGE
                  --captcha CAPTCHA

named arguments:
  -h, --help             show this help message and exit
  --challenge CHALLENGE  The challenge token taken  from the proof required
                         error.
  --captcha CAPTCHA      The captcha token from  the  solved captcha on the
                         signal website.
```

stderr:
```
```

### `signal-cli --config <CONFIG> trust --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli trust [-h] [-a | -v VERIFIED_SAFETY_NUMBER] recipient

positional arguments:
  recipient              Specify the phone  number,  for  which  to set the
                         trust.

named arguments:
  -h, --help             show this help message and exit
  -a, --trust-all-known-keys
                         Trust all known keys of  this  user, only use this
                         for testing.
  -v VERIFIED_SAFETY_NUMBER, --verified-safety-number VERIFIED_SAFETY_NUMBER, --verified-fingerprint VERIFIED_SAFETY_NUMBER
                         Specify the safety  number  of  the  key, only use
                         this  option  if  you  have  verified  the  safety
                         number.
```

stderr:
```
```

### `signal-cli --config <CONFIG> unblock --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli unblock [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [recipient [recipient ...]]

positional arguments:
  recipient              Contact number

named arguments:
  -h, --help             show this help message and exit
  -g [GROUP_ID [GROUP_ID ...]], --group-id [GROUP_ID [GROUP_ID ...]], --group [GROUP_ID [GROUP_ID ...]]
                         Group ID
```

stderr:
```
```

### `signal-cli --config <CONFIG> unregister --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli unregister [-h] [--delete-account]

named arguments:
  -h, --help             show this help message and exit
  --delete-account       Delete account  completely  from  server. CAUTION:
                         Only do this if you won't use this number again!
```

stderr:
```
```

### `signal-cli --config <CONFIG> updateAccount --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli updateAccount [-h] [-n DEVICE_NAME]
                  [--unrestricted-unidentified-sender {true,false}]
                  [--discoverable-by-number {true,false}]
                  [--number-sharing {true,false}] [-u USERNAME |
                  --delete-username]

named arguments:
  -h, --help             show this help message and exit
  -n DEVICE_NAME, --device-name DEVICE_NAME
                         Specify a name to describe this device.
  --unrestricted-unidentified-sender {true,false}
                         Enable if  anyone  should  be  able  to  send  you
                         unidentified sender messages.
  --discoverable-by-number {true,false}
                         Enable/disable   if   the    account   should   be
                         discoverable by phone number
  --number-sharing {true,false}
                         Indicates if Signal should  share its phone number
                         when sending a message.
  -u USERNAME, --username USERNAME
                         Specify a  username  that  can  then  be  used  to
                         contact this account.
  --delete-username      Delete the username associated with this account.
```

stderr:
```
```

### `signal-cli --config <CONFIG> updateConfiguration --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli updateConfiguration [-h] [--read-receipts {true,false}]
                  [--unidentified-delivery-indicators {true,false}]
                  [--typing-indicators {true,false}]
                  [--link-previews {true,false}]

named arguments:
  -h, --help             show this help message and exit
  --read-receipts {true,false}
                         Indicates if Signal should send read receipts.
  --unidentified-delivery-indicators {true,false}
                         Indicates  if  Signal   should  show  unidentified
                         delivery indicators.
  --typing-indicators {true,false}
                         Indicates  if  Signal   should   send/show  typing
                         indicators.
  --link-previews {true,false}
                         Indicates if Signal should generate link previews.
```

stderr:
```
```

### `signal-cli --config <CONFIG> updateContact --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli updateContact [-h] [-n NAME] [--given-name GIVEN_NAME]
                  [--family-name FAMILY_NAME]
                  [--nick-given-name NICK_GIVEN_NAME]
                  [--nick-family-name NICK_FAMILY_NAME] [--note NOTE]
                  [-e EXPIRATION] recipient

positional arguments:
  recipient              Contact number

named arguments:
  -h, --help             show this help message and exit
  -n NAME, --name NAME   New contact name
  --given-name GIVEN_NAME
                         New system given name
  --family-name FAMILY_NAME
                         New system family name
  --nick-given-name NICK_GIVEN_NAME
                         New nick given name
  --nick-family-name NICK_FAMILY_NAME
                         New nick family name
  --note NOTE            New note
  -e EXPIRATION, --expiration EXPIRATION
                         Set expiration time of messages (seconds)
```

stderr:
```
```

### `signal-cli --config <CONFIG> updateDevice --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli updateDevice [-h] -d DEVICE_ID -n DEVICE_NAME

named arguments:
  -h, --help             show this help message and exit
  -d DEVICE_ID, --device-id DEVICE_ID, --deviceId DEVICE_ID
                         Specify  the  device  you   want  to  update.  Use
                         listDevices to see the deviceIds.
  -n DEVICE_NAME, --device-name DEVICE_NAME
                         Specify a name to describe the given device.
```

stderr:
```
```

### `signal-cli --config <CONFIG> updateGroup --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli updateGroup [-h] [-g GROUP_ID] [-n NAME] [-d DESCRIPTION]
                  [-a AVATAR] [-m [MEMBER [MEMBER ...]]]
                  [-r [REMOVE_MEMBER [REMOVE_MEMBER ...]]]
                  [--admin [ADMIN [ADMIN ...]]]
                  [--remove-admin [REMOVE_ADMIN [REMOVE_ADMIN ...]]]
                  [--ban [BAN [BAN ...]]] [--unban [UNBAN [UNBAN ...]]]
                  [--reset-link]
                  [--link {enabled,enabled-with-approval,disabled}]
                  [--set-permission-add-member {every-member,only-admins}]
                  [--set-permission-edit-details {every-member,only-admins}]
                  [--set-permission-send-messages {every-member,only-admins}]
                  [-e EXPIRATION] [--member-label-emoji MEMBER_LABEL_EMOJI]
                  [--member-label MEMBER_LABEL]

named arguments:
  -h, --help             show this help message and exit
  -g GROUP_ID, --group-id GROUP_ID, --group GROUP_ID
                         Specify the group ID.
  -n NAME, --name NAME   Specify the new group name.
  -d DESCRIPTION, --description DESCRIPTION
                         Specify the new group description.
  -a AVATAR, --avatar AVATAR
                         Specify a new group avatar image file
  -m [MEMBER [MEMBER ...]], --member [MEMBER [MEMBER ...]]
                         Specify one or more members to add to the group
  -r [REMOVE_MEMBER [REMOVE_MEMBER ...]], --remove-member [REMOVE_MEMBER [REMOVE_MEMBER ...]]
                         Specify one or  more  members  to  remove from the
                         group
  --admin [ADMIN [ADMIN ...]]
                         Specify one or more members to make a group admin
  --remove-admin [REMOVE_ADMIN [REMOVE_ADMIN ...]]
                         Specify one or more members  to remove group admin
                         privileges
  --ban [BAN [BAN ...]]  Specify one or more  members  to  ban from joining
                         the group
  --unban [UNBAN [UNBAN ...]]
                         Specify one or  more  members  to  remove from the
                         ban list
  --reset-link           Reset group link and create new link password
  --link {enabled,enabled-with-approval,disabled}
                         Set  group  link  state,  with  or  without  admin
                         approval
  --set-permission-add-member {every-member,only-admins}
                         Set permission to add new group members
  --set-permission-edit-details {every-member,only-admins}
                         Set permission to edit group details
  --set-permission-send-messages {every-member,only-admins}
                         Set permission to send messages
  -e EXPIRATION, --expiration EXPIRATION
                         Set expiration time of messages (seconds)
  --member-label-emoji MEMBER_LABEL_EMOJI
                         Specify the emoji for the member label.
  --member-label MEMBER_LABEL
                         Specify the string for the member label.
```

stderr:
```
```

### `signal-cli --config <CONFIG> updateProfile --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli updateProfile [-h] [--given-name GIVEN_NAME]
                  [--family-name FAMILY_NAME] [--about ABOUT]
                  [--about-emoji ABOUT_EMOJI]
                  [--mobile-coin-address MOBILE_COIN_ADDRESS]
                  [--avatar AVATAR | --remove-avatar]

named arguments:
  -h, --help             show this help message and exit
  --given-name GIVEN_NAME, --name GIVEN_NAME
                         New profile (given) name
  --family-name FAMILY_NAME
                         New profile family name (optional)
  --about ABOUT          New profile about text
  --about-emoji ABOUT_EMOJI
                         New profile about emoji
  --mobile-coin-address MOBILE_COIN_ADDRESS, --mobilecoin-address MOBILE_COIN_ADDRESS
                         New  MobileCoin  address  (Base64  encoded  public
                         address)
  --avatar AVATAR        Path to new profile avatar
  --remove-avatar
```

stderr:
```
```

### `signal-cli --config <CONFIG> uploadStickerPack --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli uploadStickerPack [-h] path

positional arguments:
  path                   The path  of  the  manifest.json  or  a  zip  file
                         containing the sticker pack you wish to upload.

named arguments:
  -h, --help             show this help message and exit
```

stderr:
```
```

### `signal-cli --config <CONFIG> verify --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli verify [-h] [-p PIN] verification_code

positional arguments:
  verification_code      The verification  code  you  received  via  sms or
                         voice call.

named arguments:
  -h, --help             show this help message and exit
  -p PIN, --pin PIN      The registration lock  PIN,  that  was  set by the
                         user (Optional)
```

stderr:
```
```

### `signal-cli --config <CONFIG> version --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli version [-h]

named arguments:
  -h, --help             show this help message and exit
```

stderr:
```
```

## 5. Per-command probes, group G1

### `signal-cli --config <CONFIG> register --bogus`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli register [-h] [-v] [--captcha CAPTCHA] [--reregister]
signal-cli: error: unrecognized arguments: '--bogus'
```

### `signal-cli --config <CONFIG> register extra`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli register [-h] [-v] [--captcha CAPTCHA] [--reregister]
signal-cli: error: unrecognized arguments: 'extra'
```

### `signal-cli --config <CONFIG> -a 12345 register`

exit code: 1

stdout:
```
```

stderr:
```
Invalid account (phone number), make sure you include the country code.
```

### `signal-cli --config <CONFIG> -a 0b1c3e2a-1111-2222-3333-444455556666 register`

exit code: 1

stdout:
```
```

stderr:
```
Invalid account (phone number), make sure you include the country code.
```

### `signal-cli --config <CONFIG> verify`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli verify [-h] [-p PIN] verification_code
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> verify 1 2`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli verify [-h] [-p PIN] verification_code
signal-cli: error: unrecognized arguments: '2'
```

### `signal-cli --config <CONFIG> -a 12345 verify 123456`

exit code: 1

stdout:
```
```

stderr:
```
Invalid account (phone number), make sure you include the country code.
```

### `signal-cli --config <CONFIG> unregister --bogus`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli unregister [-h] [--delete-account]
signal-cli: error: unrecognized arguments: '--bogus'
```

### `signal-cli --config <CONFIG> -a +15555550123 unregister`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 unregister --delete-account`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> deleteLocalAccountData extra`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli deleteLocalAccountData [-h] [--ignore-registered]
signal-cli: error: unrecognized arguments: 'extra'
```

### `signal-cli --config <CONFIG> -a 12345 deleteLocalAccountData`

exit code: 1

stdout:
```
```

stderr:
```
Invalid account (phone number), make sure you include the country code.
```

### `signal-cli --config <CONFIG> updateAccount -u x --delete-username`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateAccount [-h] [-n DEVICE_NAME]
                  [--unrestricted-unidentified-sender {true,false}]
                  [--discoverable-by-number {true,false}]
                  [--number-sharing {true,false}] [-u USERNAME |
                  --delete-username]
signal-cli: error: argument --delete-username: not  allowed with argument -
u/--username
```

### `signal-cli --config <CONFIG> updateAccount --number-sharing yes`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> updateAccount --number-sharing`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateAccount [-h] [-n DEVICE_NAME]
                  [--unrestricted-unidentified-sender {true,false}]
                  [--discoverable-by-number {true,false}]
                  [--number-sharing {true,false}] [-u USERNAME |
                  --delete-username]
signal-cli: error: argument --number-sharing: expected one argument
```

### `signal-cli --config <CONFIG> -a +15555550123 updateAccount --number-sharing TRUE`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 updateAccount`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 startChangeNumber +15555550124`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> startChangeNumber a b`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli startChangeNumber [-h] [-v] [--captcha CAPTCHA] number
signal-cli: error: unrecognized arguments: 'b'
```

### `signal-cli --config <CONFIG> finishChangeNumber +15555550124`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli finishChangeNumber [-h] -v VERIFICATION_CODE [-p PIN]
                  number
signal-cli: error: argument -v/--verification-code is required
```

### `signal-cli --config <CONFIG> finishChangeNumber -v 123`

exit code: 1

stdout:
```
```

stderr:
```
2026-10-23T06:12:54.599Z [main] DEBUG org.asamk.signal.util.IOUtils - XDG_RUNTIME_DIR not set, falling back to temp dir
usage: signal-cli finishChangeNumber [-h] -v VERIFICATION_CODE [-p PIN]
                  number
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> -a +15555550123 finishChangeNumber -v 123 +15555550124`

exit code: 1

stdout:
```
```

stderr:
```
2026-10-23T06:12:58.752Z [main] DEBUG org.asamk.signal.util.IOUtils - XDG_RUNTIME_DIR not set, falling back to temp dir
2026-10-23T06:12:58.778Z [main] DEBUG org.asamk.signal.App - Starting signal-cli 0.14.7
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> updateConfiguration --read-receipts maybe`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> -a +15555550123 updateConfiguration --read-receipts True`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 updateConfiguration`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> setPin`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli setPin [-h] pin
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> setPin a b`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli setPin [-h] pin
signal-cli: error: unrecognized arguments: 'b'
```

### `signal-cli --config <CONFIG> -a +15555550123 setPin 1234`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> removePin x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli removePin [-h]
signal-cli: error: unrecognized arguments: 'x'
```

### `signal-cli --config <CONFIG> -a +15555550123 removePin`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> link extra`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli link [-h] [-n NAME]
signal-cli: error: unrecognized arguments: 'extra'
```

### `signal-cli --config <CONFIG> link -n`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli link [-h] [-n NAME]
signal-cli: error: argument -n/--name: expected one argument
```

### `signal-cli --config <CONFIG> -a +15555550123 link -n foo`

exit code: 1

stdout:
```
```

stderr:
```
You cannot specify a account (phone number) when linking
```

### `signal-cli --config <CONFIG> addDevice --uri`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli addDevice [-h] --uri URI
signal-cli: error: argument --uri: expected one argument
```

### `signal-cli --config <CONFIG> -a +15555550123 addDevice --uri sgnl://linkdevice?uuid=x&pub_key=y`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> listAccounts extra`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli listAccounts [-h]
signal-cli: error: unrecognized arguments: 'extra'
```

### `signal-cli --config <CONFIG> -a +15555550123 listAccounts`

exit code: 1

stdout:
```
```

stderr:
```
Command only works in multi-account mode
```

### `signal-cli --config <CONFIG> -o json listAccounts`

exit code: 0

stdout:
```
[]
```

stderr:
```
```

### `signal-cli --config <CONFIG> listAccounts`

exit code: 0

stdout:
```
```

stderr:
```
```

### `signal-cli --config <CONFIG> listDevices extra`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli listDevices [-h]
signal-cli: error: unrecognized arguments: 'extra'
```

### `signal-cli --config <CONFIG> -a +15555550123 listDevices`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> listDevices`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> removeDevice -d abc`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli removeDevice [-h] -d DEVICE_ID
signal-cli: error: argument  -d/--device-id/--deviceId:  could  not convert
'abc' to integer (32 bits)
```

### `signal-cli --config <CONFIG> removeDevice -d 1.5`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli removeDevice [-h] -d DEVICE_ID
signal-cli: error: argument  -d/--device-id/--deviceId:  could  not convert
'1.5' to integer (32 bits)
```

### `signal-cli --config <CONFIG> removeDevice`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli removeDevice [-h] -d DEVICE_ID
signal-cli: error: argument -d/--device-id/--deviceId is required
```

### `signal-cli --config <CONFIG> -a +15555550123 removeDevice --deviceId 2`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> updateDevice -d 2`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateDevice [-h] -d DEVICE_ID -n DEVICE_NAME
signal-cli: error: argument -n/--device-name is required
```

### `signal-cli --config <CONFIG> updateDevice -n x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateDevice [-h] -d DEVICE_ID -n DEVICE_NAME
signal-cli: error: argument -d/--device-id/--deviceId is required
```

### `signal-cli --config <CONFIG> updateDevice -d x -n y`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateDevice [-h] -d DEVICE_ID -n DEVICE_NAME
signal-cli: error: argument  -d/--device-id/--deviceId:  could  not convert
'x' to integer (32 bits)
```

### `signal-cli --config <CONFIG> -a +15555550123 updateDevice -d 2 -n y`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> submitRateLimitChallenge --challenge x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli submitRateLimitChallenge [-h] --challenge CHALLENGE
                  --captcha CAPTCHA
signal-cli: error: argument --captcha is required
```

### `signal-cli --config <CONFIG> submitRateLimitChallenge --captcha x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli submitRateLimitChallenge [-h] --challenge CHALLENGE
                  --captcha CAPTCHA
signal-cli: error: argument --challenge is required
```

### `signal-cli --config <CONFIG> -a +15555550123 submitRateLimitChallenge --challenge x --captcha y`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> version --bogus`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli version [-h]
signal-cli: error: unrecognized arguments: '--bogus'
```

### `signal-cli --config <CONFIG> -o json version`

exit code: 0

stdout:
```
{"version":"0.14.7"}
```

stderr:
```
```

### `signal-cli --config <CONFIG> version`

exit code: 0

stdout:
```
signal-cli 0.14.7
```

stderr:
```
```

### `signal-cli --config <CONFIG> -a +15555550123 version`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> register -v --bogus`

exit code: 1

stdout:
```
```

stderr:
```
2026-10-23T06:16:59.062Z [main] DEBUG org.asamk.signal.util.IOUtils - XDG_RUNTIME_DIR not set, falling back to temp dir
usage: signal-cli register [-h] [-v] [--captcha CAPTCHA] [--reregister]
signal-cli: error: unrecognized arguments: '--bogus'
```

### `signal-cli --config <CONFIG> register --voice --bogus`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli register [-h] [-v] [--captcha CAPTCHA] [--reregister]
signal-cli: error: unrecognized arguments: '--bogus'
```

## 6. Per-command probes, group G2

### `signal-cli --config <CONFIG> send -m hi --message-from-stdin +15555550123`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli send [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [-a [ATTACHMENT [ATTACHMENT ...]]]
                  [--view-once] [-e] [--mention [MENTION [MENTION ...]]]
                  [--text-style [TEXT_STYLE [TEXT_STYLE ...]]]
                  [--quote-timestamp QUOTE_TIMESTAMP]
                  [--quote-author QUOTE_AUTHOR]
                  [--quote-message QUOTE_MESSAGE]
                  [--quote-mention [QUOTE_MENTION [QUOTE_MENTION ...]]]
                  [--quote-attachment [QUOTE_ATTACHMENT [QUOTE_ATTACHMENT ...]]]
                  [--quote-text-style [QUOTE_TEXT_STYLE [QUOTE_TEXT_STYLE ...]]]
                  [--sticker STICKER] [--preview-url PREVIEW_URL]
                  [--preview-title PREVIEW_TITLE]
                  [--preview-description PREVIEW_DESCRIPTION]
                  [--preview-image PREVIEW_IMAGE]
                  [--story-timestamp STORY_TIMESTAMP]
                  [--story-author STORY_AUTHOR]
                  [--edit-timestamp EDIT_TIMESTAMP] [--no-urgent]
                  [--voice-note] [-m MESSAGE | --message-from-stdin]
                  [recipient [recipient ...]]
signal-cli:  error:  argument   --message-from-stdin:   not   allowed  with
argument -m/--message
```

### `signal-cli --config <CONFIG> send --quote-timestamp abc -m hi +15555550123`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli send [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [-a [ATTACHMENT [ATTACHMENT ...]]]
                  [--view-once] [-e] [--mention [MENTION [MENTION ...]]]
                  [--text-style [TEXT_STYLE [TEXT_STYLE ...]]]
                  [--quote-timestamp QUOTE_TIMESTAMP]
                  [--quote-author QUOTE_AUTHOR]
                  [--quote-message QUOTE_MESSAGE]
                  [--quote-mention [QUOTE_MENTION [QUOTE_MENTION ...]]]
                  [--quote-attachment [QUOTE_ATTACHMENT [QUOTE_ATTACHMENT ...]]]
                  [--quote-text-style [QUOTE_TEXT_STYLE [QUOTE_TEXT_STYLE ...]]]
                  [--sticker STICKER] [--preview-url PREVIEW_URL]
                  [--preview-title PREVIEW_TITLE]
                  [--preview-description PREVIEW_DESCRIPTION]
                  [--preview-image PREVIEW_IMAGE]
                  [--story-timestamp STORY_TIMESTAMP]
                  [--story-author STORY_AUTHOR]
                  [--edit-timestamp EDIT_TIMESTAMP] [--no-urgent]
                  [--voice-note] [-m MESSAGE | --message-from-stdin]
                  [recipient [recipient ...]]
signal-cli: error: argument --quote-timestamp:  could  not convert 'abc' to
integer (64 bits)
```

### `signal-cli --config <CONFIG> send --edit-timestamp 1.5 -m hi +15555550123`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli send [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [-a [ATTACHMENT [ATTACHMENT ...]]]
                  [--view-once] [-e] [--mention [MENTION [MENTION ...]]]
                  [--text-style [TEXT_STYLE [TEXT_STYLE ...]]]
                  [--quote-timestamp QUOTE_TIMESTAMP]
                  [--quote-author QUOTE_AUTHOR]
                  [--quote-message QUOTE_MESSAGE]
                  [--quote-mention [QUOTE_MENTION [QUOTE_MENTION ...]]]
                  [--quote-attachment [QUOTE_ATTACHMENT [QUOTE_ATTACHMENT ...]]]
                  [--quote-text-style [QUOTE_TEXT_STYLE [QUOTE_TEXT_STYLE ...]]]
                  [--sticker STICKER] [--preview-url PREVIEW_URL]
                  [--preview-title PREVIEW_TITLE]
                  [--preview-description PREVIEW_DESCRIPTION]
                  [--preview-image PREVIEW_IMAGE]
                  [--story-timestamp STORY_TIMESTAMP]
                  [--story-author STORY_AUTHOR]
                  [--edit-timestamp EDIT_TIMESTAMP] [--no-urgent]
                  [--voice-note] [-m MESSAGE | --message-from-stdin]
                  [recipient [recipient ...]]
signal-cli: error: argument --edit-timestamp:  could  not  convert '1.5' to
integer (64 bits)
```

### `signal-cli --config <CONFIG> send --story-timestamp -m hi`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli send [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [-a [ATTACHMENT [ATTACHMENT ...]]]
                  [--view-once] [-e] [--mention [MENTION [MENTION ...]]]
                  [--text-style [TEXT_STYLE [TEXT_STYLE ...]]]
                  [--quote-timestamp QUOTE_TIMESTAMP]
                  [--quote-author QUOTE_AUTHOR]
                  [--quote-message QUOTE_MESSAGE]
                  [--quote-mention [QUOTE_MENTION [QUOTE_MENTION ...]]]
                  [--quote-attachment [QUOTE_ATTACHMENT [QUOTE_ATTACHMENT ...]]]
                  [--quote-text-style [QUOTE_TEXT_STYLE [QUOTE_TEXT_STYLE ...]]]
                  [--sticker STICKER] [--preview-url PREVIEW_URL]
                  [--preview-title PREVIEW_TITLE]
                  [--preview-description PREVIEW_DESCRIPTION]
                  [--preview-image PREVIEW_IMAGE]
                  [--story-timestamp STORY_TIMESTAMP]
                  [--story-author STORY_AUTHOR]
                  [--edit-timestamp EDIT_TIMESTAMP] [--no-urgent]
                  [--voice-note] [-m MESSAGE | --message-from-stdin]
                  [recipient [recipient ...]]
signal-cli: error: argument --story-timestamp: expected one argument
```

### `signal-cli --config <CONFIG> send -m`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli send [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [-a [ATTACHMENT [ATTACHMENT ...]]]
                  [--view-once] [-e] [--mention [MENTION [MENTION ...]]]
                  [--text-style [TEXT_STYLE [TEXT_STYLE ...]]]
                  [--quote-timestamp QUOTE_TIMESTAMP]
                  [--quote-author QUOTE_AUTHOR]
                  [--quote-message QUOTE_MESSAGE]
                  [--quote-mention [QUOTE_MENTION [QUOTE_MENTION ...]]]
                  [--quote-attachment [QUOTE_ATTACHMENT [QUOTE_ATTACHMENT ...]]]
                  [--quote-text-style [QUOTE_TEXT_STYLE [QUOTE_TEXT_STYLE ...]]]
                  [--sticker STICKER] [--preview-url PREVIEW_URL]
                  [--preview-title PREVIEW_TITLE]
                  [--preview-description PREVIEW_DESCRIPTION]
                  [--preview-image PREVIEW_IMAGE]
                  [--story-timestamp STORY_TIMESTAMP]
                  [--story-author STORY_AUTHOR]
                  [--edit-timestamp EDIT_TIMESTAMP] [--no-urgent]
                  [--voice-note] [-m MESSAGE | --message-from-stdin]
                  [recipient [recipient ...]]
signal-cli: error: argument -m/--message: expected one argument
```

### `signal-cli --config <CONFIG> -a +15555550123 send -m hi +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 -o json send -m hi +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> sendAdminDelete -t 1`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendAdminDelete [-h] [-g GROUP_ID [GROUP_ID ...]]
                  [--notify-self] -a TARGET_AUTHOR -t TARGET_TIMESTAMP
                  [--story]
signal-cli: error: argument -a/--target-author is required
```

### `signal-cli --config <CONFIG> sendAdminDelete -a +15555550123`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendAdminDelete [-h] [-g GROUP_ID [GROUP_ID ...]]
                  [--notify-self] -a TARGET_AUTHOR -t TARGET_TIMESTAMP
                  [--story]
signal-cli: error: argument -t/--target-timestamp is required
```

### `signal-cli --config <CONFIG> sendAdminDelete -a +15555550123 -t 1 -g`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendAdminDelete [-h] [-g GROUP_ID [GROUP_ID ...]]
                  [--notify-self] -a TARGET_AUTHOR -t TARGET_TIMESTAMP
                  [--story]
signal-cli: error: argument -g/--group-id/--group: expected 1 argument(s)
```

### `signal-cli --config <CONFIG> sendAdminDelete -a +15555550123 -t x -g abc`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendAdminDelete [-h] [-g GROUP_ID [GROUP_ID ...]]
                  [--notify-self] -a TARGET_AUTHOR -t TARGET_TIMESTAMP
                  [--story]
signal-cli: error: argument  -t/--target-timestamp:  could  not convert 'x'
to integer (64 bits)
```

### `signal-cli --config <CONFIG> sendAdminDelete -a +15555550123 -t 1`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> sendPollCreate -q Q`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPollCreate [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] -q QUESTION [--no-multi]
                  -o OPTION [OPTION ...] [recipient [recipient ...]]
signal-cli: error: argument -o/--option is required
```

### `signal-cli --config <CONFIG> sendPollCreate -o a b`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPollCreate [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] -q QUESTION [--no-multi]
                  -o OPTION [OPTION ...] [recipient [recipient ...]]
signal-cli: error: argument -q/--question is required
```

### `signal-cli --config <CONFIG> sendPollCreate -q Q -o`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPollCreate [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] -q QUESTION [--no-multi]
                  -o OPTION [OPTION ...] [recipient [recipient ...]]
signal-cli: error: argument -o/--option: expected 1 argument(s)
```

### `signal-cli --config <CONFIG> sendPollVote --poll-timestamp 1`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPollVote [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [--poll-author POLL_AUTHOR]
                  --poll-timestamp POLL_TIMESTAMP
                  [-o [OPTION [OPTION ...]]] --vote-count VOTE_COUNT
                  [recipient [recipient ...]]
signal-cli: error: argument --vote-count is required
```

### `signal-cli --config <CONFIG> sendPollVote --vote-count 1`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPollVote [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [--poll-author POLL_AUTHOR]
                  --poll-timestamp POLL_TIMESTAMP
                  [-o [OPTION [OPTION ...]]] --vote-count VOTE_COUNT
                  [recipient [recipient ...]]
signal-cli: error: argument --poll-timestamp is required
```

### `signal-cli --config <CONFIG> sendPollVote --poll-timestamp 1 --vote-count x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPollVote [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [--poll-author POLL_AUTHOR]
                  --poll-timestamp POLL_TIMESTAMP
                  [-o [OPTION [OPTION ...]]] --vote-count VOTE_COUNT
                  [recipient [recipient ...]]
signal-cli: error: argument --vote-count: could  not convert 'x' to integer
(32 bits)
```

### `signal-cli --config <CONFIG> sendPollVote --poll-timestamp 1 --vote-count 1 -o a`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPollVote [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [--poll-author POLL_AUTHOR]
                  --poll-timestamp POLL_TIMESTAMP
                  [-o [OPTION [OPTION ...]]] --vote-count VOTE_COUNT
                  [recipient [recipient ...]]
signal-cli: error: argument -o/--option: could  not  convert 'a' to integer
(32 bits)
```

### `signal-cli --config <CONFIG> sendPollTerminate`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPollTerminate [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] --poll-timestamp POLL_TIMESTAMP
                  [recipient [recipient ...]]
signal-cli: error: argument --poll-timestamp is required
```

### `signal-cli --config <CONFIG> sendMessageRequestResponse`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendMessageRequestResponse [-h]
                  [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] --type {accept,delete}
                  [recipient [recipient ...]]
signal-cli: error: argument --type is required
```

### `signal-cli --config <CONFIG> sendMessageRequestResponse --type foo`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendMessageRequestResponse [-h]
                  [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] --type {accept,delete}
                  [recipient [recipient ...]]
signal-cli: error: argument --type:  could  not  convert 'foo' (choose from
{accept,delete})
```

### `signal-cli --config <CONFIG> sendMessageRequestResponse --type ACCEPT +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendMessageRequestResponse [-h]
                  [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] --type {accept,delete}
                  [recipient [recipient ...]]
signal-cli: error: argument  --type:  could  not  convert  'ACCEPT' (choose
from {accept,delete})
```

### `signal-cli --config <CONFIG> sendMessageRequestResponse --type accept +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> sendPaymentNotification --receipt abc`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPaymentNotification [-h] --receipt RECEIPT
                  [--note NOTE] recipient
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> sendPaymentNotification +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPaymentNotification [-h] --receipt RECEIPT
                  [--note NOTE] recipient
signal-cli: error: argument --receipt is required
```

### `signal-cli --config <CONFIG> sendPaymentNotification --receipt abc +15555550199 +15555550198`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPaymentNotification [-h] --receipt RECEIPT
                  [--note NOTE] recipient
signal-cli: error: unrecognized arguments: '+15555550198'
```

### `signal-cli --config <CONFIG> sendPinMessage -a +1`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPinMessage [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [-d PIN_DURATION] -a TARGET_AUTHOR
                  -t TARGET_TIMESTAMP [--story] [recipient [recipient ...]]
signal-cli: error: argument -t/--target-timestamp is required
```

### `signal-cli --config <CONFIG> sendPinMessage -a +1 -t 1 -d x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPinMessage [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] [-d PIN_DURATION] -a TARGET_AUTHOR
                  -t TARGET_TIMESTAMP [--story] [recipient [recipient ...]]
signal-cli: error: argument  -d/--pin-duration:  could  not  convert 'x' to
integer (32 bits)
```

### `signal-cli --config <CONFIG> sendUnpinMessage -t 1`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendUnpinMessage [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] -a TARGET_AUTHOR -t TARGET_TIMESTAMP
                  [--story] [recipient [recipient ...]]
signal-cli: error: argument -a/--target-author is required
```

### `signal-cli --config <CONFIG> sendReaction -a +1 -t 1`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendReaction [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] -e EMOJI -a TARGET_AUTHOR
                  -t TARGET_TIMESTAMP [-r] [--story]
                  [recipient [recipient ...]]
signal-cli: error: argument -e/--emoji is required
```

### `signal-cli --config <CONFIG> sendReaction -e x -a +1`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendReaction [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] -e EMOJI -a TARGET_AUTHOR
                  -t TARGET_TIMESTAMP [-r] [--story]
                  [recipient [recipient ...]]
signal-cli: error: argument -t/--target-timestamp is required
```

### `signal-cli --config <CONFIG> sendReceipt`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendReceipt [-h]
                  -t TARGET_TIMESTAMP [TARGET_TIMESTAMP ...]
                  [--type {read,viewed}] recipient
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> sendReceipt +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendReceipt [-h]
                  -t TARGET_TIMESTAMP [TARGET_TIMESTAMP ...]
                  [--type {read,viewed}] recipient
signal-cli: error: argument -t/--target-timestamp is required
```

### `signal-cli --config <CONFIG> sendReceipt -t 1`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendReceipt [-h]
                  -t TARGET_TIMESTAMP [TARGET_TIMESTAMP ...]
                  [--type {read,viewed}] recipient
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> sendReceipt -t 1 --type foo +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendReceipt [-h]
                  -t TARGET_TIMESTAMP [TARGET_TIMESTAMP ...]
                  [--type {read,viewed}] recipient
signal-cli: error: argument  --type:  invalid  choice:  'foo'  (choose from
{read,viewed})
```

### `signal-cli --config <CONFIG> sendReceipt -t 1 2 x +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendReceipt [-h]
                  -t TARGET_TIMESTAMP [TARGET_TIMESTAMP ...]
                  [--type {read,viewed}] recipient
signal-cli: error: argument  -t/--target-timestamp:  could  not convert 'x'
to integer (64 bits)
```

### `signal-cli --config <CONFIG> sendStory`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendStory [-h] -a ATTACHMENT [--no-replies] [-g GROUP_ID]
signal-cli: error: argument -a/--attachment is required
```

### `signal-cli --config <CONFIG> sendStory -a f.jpg -g a b`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendStory [-h] -a ATTACHMENT [--no-replies] [-g GROUP_ID]
signal-cli: error: unrecognized arguments: 'b'
```

### `signal-cli --config <CONFIG> sendTyping -u x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendTyping [-h] [-g [GROUP_ID [GROUP_ID ...]]] [-s]
                  [recipient [recipient ...]]
signal-cli: error: unrecognized arguments: '-u'
```

### `signal-cli --config <CONFIG> sendTyping -s extra --bogus`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendTyping [-h] [-g [GROUP_ID [GROUP_ID ...]]] [-s]
                  [recipient [recipient ...]]
signal-cli: error: unrecognized arguments: '--bogus'

Did you mean:
	--group
```

### `signal-cli --config <CONFIG> remoteDelete`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli remoteDelete [-h] -t TARGET_TIMESTAMP
                  [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [recipient [recipient ...]]
signal-cli: error: argument -t/--target-timestamp is required
```

### `signal-cli --config <CONFIG> remoteDelete -t abc`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli remoteDelete [-h] -t TARGET_TIMESTAMP
                  [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [recipient [recipient ...]]
signal-cli: error: argument -t/--target-timestamp:  could not convert 'abc'
to integer (64 bits)
```

### `signal-cli --config <CONFIG> -o json remoteDelete -t 1 +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> sendReaction`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendReaction [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] -e EMOJI -a TARGET_AUTHOR
                  -t TARGET_TIMESTAMP [-r] [--story]
                  [recipient [recipient ...]]
signal-cli: error: argument -t/--target-timestamp is required
```

### `signal-cli --config <CONFIG> sendPollTerminate --poll-timestamp x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendPollTerminate [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] --poll-timestamp POLL_TIMESTAMP
                  [recipient [recipient ...]]
signal-cli: error: argument  --poll-timestamp:  could  not  convert  'x' to
integer (64 bits)
```

### `signal-cli --config <CONFIG> sendReceipt -t 1 2 +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendReceipt [-h]
                  -t TARGET_TIMESTAMP [TARGET_TIMESTAMP ...]
                  [--type {read,viewed}] recipient
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> sendReceipt +15555550199 -t 1 2`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> sendUnpinMessage -a +1`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendUnpinMessage [-h] [-g [GROUP_ID [GROUP_ID ...]]]
                  [-u [USERNAME [USERNAME ...]]] [--note-to-self]
                  [--notify-self] -a TARGET_AUTHOR -t TARGET_TIMESTAMP
                  [--story] [recipient [recipient ...]]
signal-cli: error: argument -t/--target-timestamp is required
```

### `signal-cli --config <CONFIG> sendStory -a f.jpg -g`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendStory [-h] -a ATTACHMENT [--no-replies] [-g GROUP_ID]
signal-cli: error: argument -g/--group-id: expected one argument
```

### `signal-cli --config <CONFIG> send -a`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> -a +15555550123 sendReaction -e x -a +1 -t 1 +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

## 7. Per-command probes, group G3

### `signal-cli --config <CONFIG> receive -t abc`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli receive [-h] [-t TIMEOUT] [--max-messages MAX_MESSAGES]
                  [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]
signal-cli: error:  argument  -t/--timeout:  could  not  convert  'abc'  to
number (double precision)
```

### `signal-cli --config <CONFIG> receive --max-messages 1.5`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli receive [-h] [-t TIMEOUT] [--max-messages MAX_MESSAGES]
                  [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]
signal-cli: error: argument  --max-messages:  could  not  convert  '1.5' to
integer (32 bits)
```

### `signal-cli --config <CONFIG> -a +15555550123 receive`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 -o json receive -t 1`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> daemon --receive-mode bogus`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli daemon [-h] [--dbus] [--dbus-system]
                  [--bus-name BUS_NAME] [--socket [SOCKET]] [--tcp [TCP]]
                  [--http [HTTP]] [--no-receive-stdout]
                  [--receive-mode {on-start,on-connection,manual}]
                  [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]
signal-cli: error:  argument  --receive-mode:  could  not  convert  'bogus'
(choose from {on-start,on-connection,manual})
```

### `signal-cli --config <CONFIG> -a +15555550123 daemon`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 daemon --http`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> jsonRpc --receive-mode bogus`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli jsonRpc [-h] [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]
                  [--receive-mode {on-start,on-connection,manual}]
signal-cli: error:  argument  --receive-mode:  could  not  convert  'bogus'
(choose from {on-start,on-connection,manual})
```

### `signal-cli --config <CONFIG> -a +15555550123 jsonRpc`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> jsonRpc --no-receive-stdout`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli jsonRpc [-h] [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]
                  [--receive-mode {on-start,on-connection,manual}]
signal-cli: error: unrecognized arguments: '--no-receive-stdout'
```

### `signal-cli --config <CONFIG> joinGroup --uri`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli joinGroup [-h] --uri URI
signal-cli: error: argument --uri: expected one argument
```

### `signal-cli --config <CONFIG> -a +15555550123 joinGroup --uri https://signal.group/#x`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> updateGroup --link bogus`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateGroup [-h] [-g GROUP_ID] [-n NAME] [-d DESCRIPTION]
                  [-a AVATAR] [-m [MEMBER [MEMBER ...]]]
                  [-r [REMOVE_MEMBER [REMOVE_MEMBER ...]]]
                  [--admin [ADMIN [ADMIN ...]]]
                  [--remove-admin [REMOVE_ADMIN [REMOVE_ADMIN ...]]]
                  [--ban [BAN [BAN ...]]] [--unban [UNBAN [UNBAN ...]]]
                  [--reset-link]
                  [--link {enabled,enabled-with-approval,disabled}]
                  [--set-permission-add-member {every-member,only-admins}]
                  [--set-permission-edit-details {every-member,only-admins}]
                  [--set-permission-send-messages {every-member,only-admins}]
                  [-e EXPIRATION] [--member-label-emoji MEMBER_LABEL_EMOJI]
                  [--member-label MEMBER_LABEL]
signal-cli: error: argument --link:  invalid  choice:  'bogus' (choose from
{enabled,enabled-with-approval,disabled})
```

### `signal-cli --config <CONFIG> updateGroup --set-permission-add-member x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateGroup [-h] [-g GROUP_ID] [-n NAME] [-d DESCRIPTION]
                  [-a AVATAR] [-m [MEMBER [MEMBER ...]]]
                  [-r [REMOVE_MEMBER [REMOVE_MEMBER ...]]]
                  [--admin [ADMIN [ADMIN ...]]]
                  [--remove-admin [REMOVE_ADMIN [REMOVE_ADMIN ...]]]
                  [--ban [BAN [BAN ...]]] [--unban [UNBAN [UNBAN ...]]]
                  [--reset-link]
                  [--link {enabled,enabled-with-approval,disabled}]
                  [--set-permission-add-member {every-member,only-admins}]
                  [--set-permission-edit-details {every-member,only-admins}]
                  [--set-permission-send-messages {every-member,only-admins}]
                  [-e EXPIRATION] [--member-label-emoji MEMBER_LABEL_EMOJI]
                  [--member-label MEMBER_LABEL]
signal-cli: error:  argument  --set-permission-add-member:  invalid choice:
'x' (choose from {every-member,only-admins})
```

### `signal-cli --config <CONFIG> updateGroup -e x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateGroup [-h] [-g GROUP_ID] [-n NAME] [-d DESCRIPTION]
                  [-a AVATAR] [-m [MEMBER [MEMBER ...]]]
                  [-r [REMOVE_MEMBER [REMOVE_MEMBER ...]]]
                  [--admin [ADMIN [ADMIN ...]]]
                  [--remove-admin [REMOVE_ADMIN [REMOVE_ADMIN ...]]]
                  [--ban [BAN [BAN ...]]] [--unban [UNBAN [UNBAN ...]]]
                  [--reset-link]
                  [--link {enabled,enabled-with-approval,disabled}]
                  [--set-permission-add-member {every-member,only-admins}]
                  [--set-permission-edit-details {every-member,only-admins}]
                  [--set-permission-send-messages {every-member,only-admins}]
                  [-e EXPIRATION] [--member-label-emoji MEMBER_LABEL_EMOJI]
                  [--member-label MEMBER_LABEL]
signal-cli: error:  argument  -e/--expiration:  could  not  convert  'x' to
integer (32 bits)
```

### `signal-cli --config <CONFIG> -a +15555550123 updateGroup -n test`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> quitGroup`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli quitGroup [-h] -g GROUP_ID [--delete]
                  [--admin [ADMIN [ADMIN ...]]]
signal-cli: error: argument -g/--group-id/--group is required
```

### `signal-cli --config <CONFIG> -a +15555550123 quitGroup -g abc`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> listGroups -x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli listGroups [-h] [-d] [-g [GROUP_ID [GROUP_ID ...]]]
signal-cli: error: unrecognized arguments: '-x'
```

### `signal-cli --config <CONFIG> -a +15555550123 listGroups -d`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> listContacts --blocked`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli listContacts [-h] [-a] [--blocked {true,false}]
                  [--name NAME] [--detailed] [--internal]
                  [recipient [recipient ...]]
signal-cli: error: argument --blocked: expected one argument
```

### `signal-cli --config <CONFIG> -a +15555550123 listContacts --blocked maybe`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> listIdentities -n`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli listIdentities [-h] [-n NUMBER]
signal-cli: error: argument -n/--number: expected one argument
```

### `signal-cli --config <CONFIG> -a +15555550123 listIdentities`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> updateProfile --avatar a --remove-avatar`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateProfile [-h] [--given-name GIVEN_NAME]
                  [--family-name FAMILY_NAME] [--about ABOUT]
                  [--about-emoji ABOUT_EMOJI]
                  [--mobile-coin-address MOBILE_COIN_ADDRESS]
                  [--avatar AVATAR | --remove-avatar]
signal-cli: error: argument --remove-avatar:  not  allowed with argument --
avatar
```

### `signal-cli --config <CONFIG> -a +15555550123 updateProfile --given-name x`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> updateContact`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateContact [-h] [-n NAME] [--given-name GIVEN_NAME]
                  [--family-name FAMILY_NAME]
                  [--nick-given-name NICK_GIVEN_NAME]
                  [--nick-family-name NICK_FAMILY_NAME] [--note NOTE]
                  [-e EXPIRATION] recipient
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> updateContact +1 -e x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli updateContact [-h] [-n NAME] [--given-name GIVEN_NAME]
                  [--family-name FAMILY_NAME]
                  [--nick-given-name NICK_GIVEN_NAME]
                  [--nick-family-name NICK_FAMILY_NAME] [--note NOTE]
                  [-e EXPIRATION] recipient
signal-cli: error:  argument  -e/--expiration:  could  not  convert  'x' to
integer (32 bits)
```

### `signal-cli --config <CONFIG> -a +15555550123 updateContact +15555550124 -n x`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> removeContact --hide --forget +1`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli removeContact [-h] [--hide | --forget] recipient
signal-cli: error: argument --forget: not allowed with argument --hide
```

### `signal-cli --config <CONFIG> removeContact +1 +2`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli removeContact [-h] [--hide | --forget] recipient
signal-cli: error: unrecognized arguments: '+2'
```

### `signal-cli --config <CONFIG> -a +15555550123 removeContact +15555550124`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> receive --help`

exit code: 0

stdout:
```
usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]

named arguments:
  -h, --help             show this help message and exit
  -v, --verbose
  --log-file LOG_FILE
  --scrub-log
usage: signal-cli receive [-h] [-t TIMEOUT] [--max-messages MAX_MESSAGES]
                  [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]

named arguments:
  -h, --help             show this help message and exit
  -t TIMEOUT, --timeout TIMEOUT
                         Number  of  seconds  to   wait  for  new  messages
                         (negative values disable timeout)
  --max-messages MAX_MESSAGES
                         Maximum number  of  messages  to  receive,  before
                         returning.
  --ignore-attachments   Don’t download attachments of received messages.
  --ignore-stories       Don’t receive story messages from the server.
  --ignore-avatars       Don't download avatars of received messages.
  --ignore-stickers      Don't download sticker packs of received messages.
  --send-read-receipts   Send read receipts for  all incoming data messages
                         (in addition to the default delivery receipts)
```

stderr:
```
```

## 8. Per-command probes, group G4

### `signal-cli --config <CONFIG> trust`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli trust [-h] [-a | -v VERIFIED_SAFETY_NUMBER] recipient
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> trust -a -v 123 +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
2026-10-23T06:12:27.929Z [main] DEBUG org.asamk.signal.util.IOUtils - XDG_RUNTIME_DIR not set, falling back to temp dir
usage: signal-cli trust [-h] [-a | -v VERIFIED_SAFETY_NUMBER] recipient
signal-cli:   error:    argument    -v/--verified-safety-number/--verified-
fingerprint: not allowed with argument -a/--trust-all-known-keys
```

### `signal-cli --config <CONFIG> -a +15555550123 trust +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 trust -a +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 block +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 unblock -g abc`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> sendContacts extra`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendContacts [-h]
signal-cli: error: unrecognized arguments: 'extra'
```

### `signal-cli --config <CONFIG> sendSyncRequest extra`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli sendSyncRequest [-h]
signal-cli: error: unrecognized arguments: 'extra'
```

### `signal-cli --config <CONFIG> -a +15555550123 sendContacts`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 sendSyncRequest`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> uploadStickerPack`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli uploadStickerPack [-h] path
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> uploadStickerPack a b`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli uploadStickerPack [-h] path
signal-cli: error: unrecognized arguments: 'b'
```

### `signal-cli --config <CONFIG> -a +15555550123 uploadStickerPack /nonexistent/manifest.json`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 listStickerPacks`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -o json listStickerPacks`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> addStickerPack --uri`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli addStickerPack [-h] --uri URI [URI ...]
signal-cli: error: argument --uri: expected 1 argument(s)
```

### `signal-cli --config <CONFIG> -a +15555550123 addStickerPack --uri https://signal.art/addstickers/#pack_id=00&pack_key=00`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> getAttachment`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli getAttachment [-h] --id ID (--recipient RECIPIENT |
                  -g GROUP_ID)
signal-cli: error: argument --id is required
```

### `signal-cli --config <CONFIG> getAttachment --id x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli getAttachment [-h] --id ID (--recipient RECIPIENT |
                  -g GROUP_ID)
signal-cli: error:  one  of  the  arguments  --recipient  -g/--group-id  is
required
```

### `signal-cli --config <CONFIG> getAttachment --id x --recipient +15555550199 -g abc`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli getAttachment [-h] --id ID (--recipient RECIPIENT |
                  -g GROUP_ID)
signal-cli: error: argument  -g/--group-id:  not  allowed  with argument --
recipient
```

### `signal-cli --config <CONFIG> -a +15555550123 getAttachment --id x -g abc`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> getAvatar`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli getAvatar [-h] (-c CONTACT | -p PROFILE | -g GROUP_ID)
signal-cli: error: one  of  the  arguments  -c/--contact -p/--profile -g/--
group-id is required
```

### `signal-cli --config <CONFIG> getAvatar -c +15555550199 -p +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli getAvatar [-h] (-c CONTACT | -p PROFILE | -g GROUP_ID)
signal-cli: error: argument -p/--profile:  not  allowed with argument -c/--
contact
```

### `signal-cli --config <CONFIG> -a +15555550123 getAvatar -g abc`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> getSticker`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli getSticker [-h] --pack-id PACK_ID --sticker-id STICKER_ID
signal-cli: error: argument --sticker-id is required
```

### `signal-cli --config <CONFIG> getSticker --pack-id ab --sticker-id x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli getSticker [-h] --pack-id PACK_ID --sticker-id STICKER_ID
signal-cli: error: argument --sticker-id: could  not convert 'x' to integer
(32 bits)
```

### `signal-cli --config <CONFIG> -a +15555550123 getSticker --pack-id ab --sticker-id 1`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 getUserStatus +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 getUserStatus --username u.01`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> startCall`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli startCall [-h] recipient
signal-cli: error: too few arguments
```

### `signal-cli --config <CONFIG> startCall +15555550199 +15555550198`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli startCall [-h] recipient
signal-cli: error: unrecognized arguments: '+15555550198'
```

### `signal-cli --config <CONFIG> -a +15555550123 startCall +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> acceptCall`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli acceptCall [-h] --call-id CALL_ID
signal-cli: error: argument --call-id is required
```

### `signal-cli --config <CONFIG> acceptCall --call-id abc`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli acceptCall [-h] --call-id CALL_ID
signal-cli: error: argument --call-id: could  not  convert 'abc' to integer
(64 bits)
```

### `signal-cli --config <CONFIG> acceptCall --call-id 99999999999999999999`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli acceptCall [-h] --call-id CALL_ID
signal-cli:    error:    argument    --call-id:     could    not    convert
'99999999999999999999' to integer (64 bits)
```

### `signal-cli --config <CONFIG> -a +15555550123 acceptCall --call-id 1`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 acceptCall --call-id -5`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> hangupCall --call-id x`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli hangupCall [-h] --call-id CALL_ID
signal-cli: error: argument --call-id:  could  not  convert  'x' to integer
(64 bits)
```

### `signal-cli --config <CONFIG> -a +15555550123 hangupCall --call-id 1`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> rejectCall --call-id 1.5`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli rejectCall [-h] --call-id CALL_ID
signal-cli: error: argument --call-id: could  not  convert '1.5' to integer
(64 bits)
```

### `signal-cli --config <CONFIG> -a +15555550123 rejectCall --call-id 1`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> -a +15555550123 listCalls`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> listCalls extra`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli listCalls [-h]
signal-cli: error: unrecognized arguments: 'extra'
```

### `signal-cli --config <CONFIG> getSticker --pack-id ab --sticker-id 99999999999`

exit code: 1

stdout:
```
```

stderr:
```
usage: signal-cli getSticker [-h] --pack-id PACK_ID --sticker-id STICKER_ID
signal-cli: error: argument --sticker-id:  could  not convert '99999999999'
to integer (32 bits)
```

### `signal-cli --config <CONFIG> -a +15555550123 trust -v 123 +15555550199`

exit code: 1

stdout:
```
```

stderr:
```
2026-10-23T06:15:45.156Z [main] DEBUG org.asamk.signal.util.IOUtils - XDG_RUNTIME_DIR not set, falling back to temp dir
2026-10-23T06:15:45.178Z [main] DEBUG org.asamk.signal.App - Starting signal-cli 0.14.7
User +15555550123 is not registered.
```

### `signal-cli --config <CONFIG> getUserStatus --username`

exit code: 1

stdout:
```
```

stderr:
```
No local users found, you first need to register or link an account
```

### `signal-cli --config <CONFIG> -a +15555550123 block`

exit code: 1

stdout:
```
```

stderr:
```
User +15555550123 is not registered.
```

