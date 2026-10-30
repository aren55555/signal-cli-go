# Service environments: staging vs production (signal-cli 0.14.7)

This spec lists every value that differs between signal-cli 0.14.7's two
service environments, **staging** (`STAGING`) and **production**
(`LIVE`), and the values that are the same in both where a reader might
expect them to differ. It is written so the implementation role can add
production alongside the existing staging configuration. The
machine-readable copy is `testdata/vectors/environments.json` (§13).

Nothing here was observed on the wire. No Signal server (staging or
production) was contacted. Every value comes from the upstream source at
the tags below. Where marked "jar", it was also checked against the
reference distribution in `/work/third_party/signal-cli-0.14.7/lib/`.

## Sources

| Ref | What | URL @ version |
|-----|------|---------------|
| E1 | signal-cli | https://github.com/AsamK/signal-cli @ v0.14.7 (`b01b6b370dc063599a1a2b9fde0f5ff4e2d78fe8`): `lib/src/main/java/org/asamk/signal/manager/config/{LiveConfig,StagingConfig,ServiceConfig,ServiceEnvironmentConfig,WhisperTrustStore}.java`, `lib/src/main/resources/org/asamk/signal/manager/config/whisper.store`, `lib/src/main/java/org/asamk/signal/manager/api/ServiceEnvironment.java`, `.../manager/internal/SignalDependencies.java`, `.../manager/helper/RecipientHelper.java`, `.../manager/storage/SignalAccount.java`, `.../manager/storage/accounts/{AccountsStore,AccountsStorage}.java`, `.../manager/SignalAccountFiles.java`, `src/main/java/org/asamk/signal/{App,ServiceEnvironmentCli}.java`, `src/main/java/org/asamk/signal/util/{CommandUtil,SendMessageResultUtils}.java`, `man/signal-cli.1.adoc` |
| E2 | signal-service-java (Turasa fork) | https://github.com/Turasa/libsignal-service-java @ v2.15.3_unofficial_151 (`66956cb4a0394d50cee3590209baf58312084a44`): `core/network/src/main/java/org/signal/network/config/SignalServiceConfiguration.kt`, `lib/libsignal-service/src/main/java/org/whispersystems/signalservice/internal/push/PushServiceSocket.java`, `.../api/provisioning/ProvisioningSocket.kt`, `.../api/svr/Svr2Socket.kt`, `.../api/cds/CdsiV2Service.java` |
| E3 | libsignal | https://github.com/signalapp/libsignal @ v0.99.1 (`97801d22dcf9f5bf714f7b8fa3212cdc973ae1c8`): `rust/net/src/env.rs`, `rust/net/src/certs.rs`, `rust/net/res/{signal.cer,signal-ed25519.cer}`, `rust/attest/src/constants.rs`, `java/client/src/main/java/org/signal/libsignal/net/Network.java` |
| E4 | Reference distribution (jar) | `/work/third_party/signal-cli-0.14.7/lib/`: `libsignal-cli-0.14.7.jar` (`javap -constants` of `LiveConfig`/`StagingConfig`; byte search of all classes), `signal-cli-0.14.7.jar` (byte search for captcha URLs), `signal-service-java-`, `signal-network-`, `core-network-`, `util-jvm-`, `models-jvm-2.15.3_unofficial_151.jar` (byte search: no hostnames at all), `libsignal-client-0.99.1.jar` → `libsignal_jni_amd64.so` (string and hex search), `bcprov-jdk18on-1.85.jar` (to list `whisper.store` with `keytool -storetype BKS`) |

Line numbers refer to the files at the commits above.

**Encodings.** Values are given in the encoding the upstream
configuration uses: base64 (standard alphabet, with padding) for the
sealed-sender trust roots and zkgroup parameters (E1 decodes them with
`java.util.Base64.getDecoder()`); lowercase hex for MRENCLAVE values and
key-transparency keys (E1 string constants; E3 `hex!` literals); decimal
for Raft group IDs; colon-free lowercase hex SHA-256 for certificate
fingerprints (our choice; upstream stores the certificates themselves).

## 1. How the environment is chosen

| Item | Staging | Production | Source |
|------|---------|------------|--------|
| CLI value of `--service-environment` | `staging` | `live` | E1 `ServiceEnvironmentCli.java` lines 11-34: input is trimmed and lower-cased, so `LIVE`, ` Live ` etc. are accepted; anything else fails with `Invalid service-environment: <value>` |
| Default | | `live` is the default, unless the global config file supplies `serviceEnvironment` | E1 `App.java` lines 100-103 |
| CLI → library enum | `STAGING` | `LIVE` | E1 `App.java` lines 249-252 (`live` → `LIVE`, otherwise `STAGING`) |
| Library enum | `ServiceEnvironment.STAGING` | `ServiceEnvironment.LIVE` | E1 `api/ServiceEnvironment.java` lines 3-6 |
| Account file, key `serviceEnvironment` | `"STAGING"` | `"LIVE"` | E1 `SignalAccount.java` line 1003 (written with the enum's `name()`), lines 505 and 588-589 (read with `valueOf`). When the key is absent (legacy file), the account counts as `LIVE` (lines 591-592) |
| `accounts.json`, key `environment` | `"STAGING"` | `"LIVE"` | E1 `AccountsStore.java` lines 281-286. `null` matches any environment (lines 65-140), as `spec/storage/signal-cli-data-dir.md` already says |
| libsignal-net environment | `Network.Environment.STAGING` (native value 0) | `Network.Environment.PRODUCTION` (native value 1) | E1 `LiveConfig.java`/`StagingConfig.java` line 57; E3 `Network.java` lines 24-26 |
| Config class selected | `StagingConfig` | `LiveConfig` | E1 `ServiceConfig.java` lines 66-69 |

Opening an account whose file says one environment with the other fails
with `Account is registered in another environment: <ENUM>` (E1
`SignalAccountFiles.java` lines 152-153). That is already in
`signal-cli-data-dir.md`.

## 2. Chat service

signal-cli talks to the chat service in two ways (`transport.md` §1):
libsignal-net for the authenticated and unauthenticated chat WebSockets,
and OkHttp REST for registration and the 1:1 send fallback. The OkHttp
provisioning socket uses the REST base URL.

| Item | Staging | Production | Used for | Source |
|------|---------|------------|----------|--------|
| REST base URL | `https://chat.staging.signal.org` | `https://chat.signal.org` | registration and verification-session calls; REST fallback for 1:1 sends; base of the provisioning URL | E1 `StagingConfig.java`/`LiveConfig.java` line 36, used at line 62 as the single `SignalServiceUrl`; jar |
| libsignal-net chat host (WebSocket over HTTP/2, both the authenticated and the unauthenticated socket) | `grpc.chat.staging.signal.org` | `grpc.chat.signal.org` | message pipe, sealed and unsealed sends, keys, profiles, certificates, account calls | E3 `env.rs` lines 52 (prod) and 77 (staging), selected by `PROD`/`STAGING` lines 865-866 and 829-830; jar (`libsignal_jni_amd64.so` contains both strings) |
| Chat WebSocket path | `/v1/websocket/` | same | | E3 `env.rs` line 906 |
| Port | 443 | 443 | | E3 `env.rs` line 36 |
| Provisioning (linking) URL | `wss://chat.staging.signal.org/v1/websocket/provisioning/` | `wss://chat.signal.org/v1/websocket/provisioning/` | the secondary device's provisioning socket (OkHttp, not libsignal-net) | E2 `ProvisioningSocket.kt` lines 114 and 132-135: the REST base URL with `https://` replaced by `wss://`, plus the path |

**gRPC.** The host name `grpc.chat…` is only a name. signal-cli 0.14.7
speaks no gRPC to it; it carries the WebSocket (`transport.md` §3.6). So
the "gRPC chat host" and the "WebSocket host" are the same value. The JSON
gives it once as `chat_ws_host` and repeats it as `chat_grpc_host`.

**Fronting / alternative hosts.** signal-cli configures **one** URL per
service, with no alternate `Host` header, in both environments (E1 lines
62-71: one array element each; `SignalServiceUrl`/`SignalCdnUrl` built
from URL and truststore only). The last constructor argument, `censored`,
is `false` in both (line 79). libsignal-net carries domain-fronting
routes for each environment, which signal-cli never enables
(`transport.md` §3.2 explains why). For completeness, the material that
differs:

| Item | Staging | Production | Source |
|------|---------|------------|--------|
| Fastly reflector host | `reflector-staging-signal.global.ssl.fastly.net` | `reflector-signal.global.ssl.fastly.net` | E3 `env.rs` lines 254-274 |
| Google reflector host | `reflector-nrgwuv7kwq-uc.a.run.app` | same | E3 `env.rs` lines 276-287 |
| Chat proxy path prefix | `/service-staging` | `/service` | E3 `env.rs` lines 58-61, 82-85 |
| CDSI proxy path prefix | `/cdsi-staging` | `/cdsi` | E3 `env.rs` lines 101-103, 119-121 |
| SVR2 proxy path prefix | `/svr2-staging` | `/svr2` | E3 `env.rs` lines 137-139, 171-173 |
| SVR-B proxy path prefix | `/svrb-staging` | `/svrb` | E3 `env.rs` lines 201-203, 231-233 |
| TLS-tunnel reflector path | `/tls-tunnel-staging` | `/tls-tunnel` | E3 `env.rs` lines 289-309 |

**Not used by default** in either environment. A Go client need not
implement any of it.

**Static DNS fallback.** libsignal-net falls back to a built-in IP table
when name resolution fails (`transport.md` §3.1). This is a resilience
measure only. The production chat entries are IPv4 `76.223.66.180`,
`15.197.251.99` and IPv6 `2600:9000:a507:ab6d:575d:9d9f:64af:7a5a`,
`2600:9000:a61f:527c:8e4a:4b48:bbfd:c9bb` (E3 `env.rs` lines 43-47). The
staging entries are those already in `transport.md` §3.1 (E3 `env.rs`
lines 67-74). The full table (JSON key `static_ip_fallback`):

| Host | Static IPs, in source order | E3 `env.rs` lines |
|------|-----------------------------|-------------------|
| `grpc.chat.staging.signal.org` | `99.83.186.178`, `166.117.39.117`, `2600:9000:a61f:527c:a456:24e3:e151:80d`, `2600:9000:a507:ab6d:8674:deea:e137:6b31` | 67-74 |
| `grpc.chat.signal.org` | `76.223.66.180`, `15.197.251.99`, `2600:9000:a507:ab6d:575d:9d9f:64af:7a5a`, `2600:9000:a61f:527c:8e4a:4b48:bbfd:c9bb` | 43-47 |
| `cdsi.staging.signal.org` | `104.43.162.137`, `2603:1030:7::732` | 122-123 |
| `cdsi.signal.org` | `40.122.45.194`, `2603:1030:7::1` | 104-105 |
| `svr2.staging.signal.org` | `104.43.134.192`, `20.253.229.239`, `157.55.188.67`, `20.127.86.118`, `20.186.175.196`, `2603:1030:20e:31::20e`, `2603:1030:403:29::7f`, `2603:1030:b:2c::26`, `2603:1030:800:5::bfee:ab23`, `2603:1030:a04:16::3a` | 175-187 |
| `svr2.signal.org` | `20.236.21.158`, `20.104.52.125`, `20.9.45.98`, `20.66.40.69`, `20.119.62.85`, `20.65.43.198`, `13.84.216.212`, `2603:1030:20e:33::6`, `2603:1030:408:3::1d`, `2603:1030:b:2a::12`, `2603:1030:803:4::65`, `2a01:111:f100:3000::a83e:1208`, `2603:1030:c04:1e::31c`, `2603:1030:f00::17` | 141-157 |

The SVR2 entries apply only to libsignal-net connections. signal-cli's
SVR2 traffic goes through OkHttp (§6), which uses the system resolver,
so for SVR2 these IPs are not used by signal-cli at all.

## 3. Storage service and groups

| Item | Staging | Production | Used for | Source |
|------|---------|------------|----------|--------|
| Storage base URL | `https://storage-staging.signal.org` | `https://storage.signal.org` | storage service (`/v1/storage…`) **and** groups v2 (`/v2/groups…`) | E1 line 40, used at line 69; E2 `PushServiceSocket.java` lines 172-178 (group paths), 225 (storage clients from `signalStorageUrls`), 1349-1358 (`makeStorageRequest`), 1838-1839 (group calls go through `makeStorageRequest`); jar |

There is no separate groups URL. Note the staging name uses a hyphen
(`storage-staging`), unlike the chat, CDSI and SVR2 names
(`*.staging.signal.org`).

## 4. CDNs

| CDN number | Staging | Production | Source |
|------------|---------|------------|--------|
| 0 | `https://cdn-staging.signal.org` | `https://cdn.signal.org` | E1 line 37, map entry at lines 63-64 |
| 2 | `https://cdn2-staging.signal.org` | `https://cdn2.signal.org` | E1 line 38, lines 65-66 |
| 3 | `https://cdn3-staging.signal.org` | `https://cdn3.signal.org` | E1 line 39, lines 67-68 |

No other CDN numbers are configured (the map has exactly the keys 0, 2,
3). An upload form naming another CDN number fails in E2
(`PushServiceSocket.java` lines 806-815, "Unknown CDN version"). Jar
confirmed.

## 5. Contact discovery (CDSI)

CDSI goes through libsignal-net (`Network.cdsiLookup`, E2
`CdsiV2Service.java` line 47; E1 `RecipientHelper.java` line 255 passes
the account's `Network`). So host and enclave come from **libsignal's**
environment, not from signal-cli's config.

| Item | Staging | Production | Source |
|------|---------|------------|--------|
| Host | `cdsi.staging.signal.org` | `cdsi.signal.org` | E3 `env.rs` lines 93 and 111, selected at lines 868-872 / 832-836. signal-cli's own `SIGNAL_CDSI_URL` (`https://cdsi.staging.signal.org` / `https://cdsi.signal.org`, E1 line 41) names the same hosts, but nothing in the CDSI path reads it |
| URL | `wss://cdsi.staging.signal.org/v1/6d9b9649fa3a337754a98059c66d48ac77aaca5299d3b27d6ed1e646c7c81c0a/discovery` | `wss://cdsi.signal.org/v1/15637fa1e54fe655176d3df1a9f94b87c01ed377acaa570682dc5d72c95ef07b/discovery` | path form from `cdsi.md` §2 |
| MRENCLAVE | `6d9b9649fa3a337754a98059c66d48ac77aaca5299d3b27d6ed1e646c7c81c0a` | `15637fa1e54fe655176d3df1a9f94b87c01ed377acaa570682dc5d72c95ef07b` | E3 `constants.rs` lines 69-70 (`ENCLAVE_ID_CDSI_STAGING`, `ENCLAVE_ID_CDSI_PROD`), used by `env.rs` lines 311-314 and 358-361; jar (both 32-byte values present in `libsignal_jni_amd64.so`) |
| Accepted SW advisories | `INTEL-SA-00615`, `INTEL-SA-00657` | same | E3 `constants.rs` lines 14-17 (`common`), lines 69-70 |
| Raft config | none (CDS2 handshake) | none | E3 `env.rs` lines 313, 360 |
| TLS | min TLS 1.3, HTTP/1.1, Signal roots (§9) | same | E3 `env.rs` lines 90-124 |

**Unused config value.** signal-cli's `CDSI_MRENCLAVE`
(`0f6fd79cdfdaa5b2e6337f534d3baf999318b0c462a7ac1f41297a3e4b424a57`, E1
line 32) is **the same in both config classes**. It is stored in
`ServiceEnvironmentConfig.cdsiMrenclave` but never read: a byte search
of every class in `libsignal-cli-0.14.7.jar` finds the name
`cdsiMrenclave` only in `ServiceEnvironmentConfig.class` itself (E4). It
is also absent from libsignal's enclave table (E3 `constants.rs`) and
from `libsignal_jni_amd64.so` (E4). An enclave presenting it would fail
attestation. Do not use it. This agrees with `attestation.md` §2.1.

Other attestation inputs (the SGX TCB evaluation-data minimum, Intel
roots, QE identity checks) are the same for both environments
(`attestation.md`; E3 `constants.rs` line 11).

## 6. SVR2 and SVR-B

SVR2 traffic uses signal-cli's own list of enclaves and its own URL
(OkHttp WebSocket in E2 `Svr2Socket.kt`). libsignal checks the attested
Raft configuration against its table keyed by MRENCLAVE (E3
`constants.rs` lines 169-182). The table has both production entries.

| Item | Staging | Production | Source |
|------|---------|------------|--------|
| Base URL | `https://svr2.staging.signal.org` | `https://svr2.signal.org` | E1 line 42, used at line 71 (one `SignalSvr2Url`, no host header) |
| Connection URL | `<base>/v1/<MRENCLAVE hex>`, handed to OkHttp's WebSocket client, which maps `https` to `wss` | same | E2 `Svr2Socket.kt` lines 35-41 |
| Full connection URLs, in order | `https://svr2.staging.signal.org/v1/3c699f4975aaa3d172c0aad042f94f031b2b03e10b9c19a45116a01693d83302`, `https://svr2.staging.signal.org/v1/97f151f6ed078edbbfd72fa9cae694dcc08353f1f5e8d9ccd79a971b10ffc535` | `https://svr2.signal.org/v1/ced8217b26228e4b210c985786999d095c4958a94faf37b14acaf25c4cbb02a4`, `https://svr2.signal.org/v1/1240acbd4aa26974184844c8a46b1022d3957ac8a76c1fd8f5b1a15141ee0708` | as above |
| Enclaves, in order | 1. `3c699f4975aaa3d172c0aad042f94f031b2b03e10b9c19a45116a01693d83302`; 2. `97f151f6ed078edbbfd72fa9cae694dcc08353f1f5e8d9ccd79a971b10ffc535` | 1. `ced8217b26228e4b210c985786999d095c4958a94faf37b14acaf25c4cbb02a4`; 2. `1240acbd4aa26974184844c8a46b1022d3957ac8a76c1fd8f5b1a15141ee0708` | E1 lines 33-34 (`SVR2_MRENCLAVE`, `SVR2_MRENCLAVE_LEGACY`), list order at line 97; E1 `SignalDependencies.java` lines 376-382 keep that order; jar |

Per-enclave parameters (E3 `constants.rs`; jar: the four production
MRENCLAVEs and the production group IDs, as little-endian u64, are
present in `libsignal_jni_amd64.so`):

| Env | Role | MRENCLAVE | libsignal name | min_voting_replicas | max_voting_replicas | super_majority | group_id | db_version | attestation_timeout | simulated | advisories | constants.rs lines |
|-----|------|-----------|----------------|---|---|---|---|---|---|---|---|---|
| staging | current | `3c699f4975aaa3d172c0aad042f94f031b2b03e10b9c19a45116a01693d83302` | `SVR2_2026Q2_STAGING` | 3 | 9 | 0 | 5138641357881452604 | 2 | 604800 | false | `INTEL-SA-00615`, `INTEL-SA-00657` | 64, 127-135 |
| staging | legacy | `97f151f6ed078edbbfd72fa9cae694dcc08353f1f5e8d9ccd79a971b10ffc535` | `SVR2_2026Q1_STAGING` | 3 | 9 | 0 | 2330628069874851020 | 2 | 604800 | false | same | 60, 87-95 |
| production | current | `ced8217b26228e4b210c985786999d095c4958a94faf37b14acaf25c4cbb02a4` | `SVR2_2026Q2_PROD` | 4 | 13 | 2 | 11311619198250676136 | 2 | 604800 | false | same | 65, 137-145 |
| production | legacy | `1240acbd4aa26974184844c8a46b1022d3957ac8a76c1fd8f5b1a15141ee0708` | `SVR2_2026Q1_PROD` | 4 | 13 | 2 | 2076725645304009823 | 2 | 604800 | false | same | 61, 97-105 |

Notes:

- libsignal's own production SVR2 environment lists the same two
  enclaves (current 2026Q2, previous 2026Q1; E3 `env.rs` lines 873-884).
  Its staging environment lists only 2026Q2 (lines 837-844), so
  signal-cli's staging legacy entry comes from signal-cli alone.
- libsignal's table also has `SVR2_2025Q3_PROD`
  (`29cd63c87bea751e3bfd0fbd401279192e2e5c99948b4ee9437eafc4968355fb`,
  4/13/2, group_id 10263621230883829694; E3 lines 59, 77-85). signal-cli
  0.14.7 does **not** use it. Mentioned only so it is not mistaken for a
  missing value.
- SVR2 TLS: OkHttp with `whisper.store` (§9), in both environments.

**SVR-B (backups).** libsignal's environments configure SVR-B. signal-cli
0.14.7 never reaches it: a byte search of its jars finds no SVR-B or
backup-service API use (E4; `transport.md` §2 says the same for staging).
**Not used.** For reference only (E3 `env.rs` lines 193/223,
847-860/885-899; `constants.rs` lines 62-67, 107-165):

| Env | Host | Enclaves (current, in order) | group_id | min/max/super_majority | db_version |
|-----|------|------------------------------|----------|-----|---|
| staging | `svrb.staging.signal.org` | `3c699f4975aaa3d172c0aad042f94f031b2b03e10b9c19a45116a01693d83302` (2026Q2), `97f151f6ed078edbbfd72fa9cae694dcc08353f1f5e8d9ccd79a971b10ffc535` (2026Q1) | 11614651745041226414, 1416305463306398324 | 3/9/0 | 4 |
| production | `svrb.signal.org` | `2048e20fcd07d0992c4907e8e04c5a85f1f993d195004c7342675343ca2e524b` (2026Q2), `bee62050df1072e3d9fdf7660bfaf4e4b71f5622db9de8b30fc5f4b9852d8359` (2026Q1) | 15457520608692442134, 16652830871035963553 | 4/13/2 | 4 |

## 7. zkgroup public parameters

Exact base64 strings as configured (E1 lines 49-55; jar `javap
-constants` gives identical strings). They are passed to
`SignalServiceConfiguration` (E1 lines 76-78; E2
`SignalServiceConfiguration.kt` lines 21-23).

| Env | Field | Decoded bytes | base64 |
|-----|-------|---------------|--------|
| staging | `zkGroupServerPublicParams` | 673 | `ABSY21VckQcbSXVNCGRYJcfWHiAMZmpTtTELcDmxgdFbtp/bWsSxZdMKzfCp8rvIs8ocCU3B37fT3r4Mi5qAemeGeR2X+/YmOGR5ofui7tD5mDQfstAI9i+4WpMtIe8KC3wU5w3Inq3uNWVmoGtpKndsNfwJrCg0Hd9zmObhypUnSkfYn2ooMOOnBpfdanRtrvetZUayDMSC5iSRcXKpdlukrpzzsCIvEwjwQlJYVPOQPj4V0F4UXXBdHSLK05uoPBCQG8G9rYIGedYsClJXnbrgGYG3eMTG5hnx4X4ntARBgELuMWWUEEfSK0mjXg+/2lPmWcTZWR9nkqgQQP0tbzuiPm74H2wMO4u1Wafe+UwyIlIT9L7KLS19Aw8r4sPrXZSSsOZ6s7M1+rTJN0bI5CKY2PX29y5Ok3jSWufIKcgKOnWoP67d5b2du2ZVJjpjfibNIHbT/cegy/sBLoFwtHogVYUewANUAXIaMPyCLRArsKhfJ5wBtTminG/PAvuBdJ70Z/bXVPf8TVsR292zQ65xwvWTejROW6AZX6aqucUjlENAErBme1YHmOSpU6tr6doJ66dPzVAWIanmO/5mgjNEDeK7DDqQdB1xd03HT2Qs2TxY3kCK8aAb/0iM0HQiXjxZ9HIgYhbtvGEnDKW5ILSUydqH/KBhW4Pb0jZWnqN/YgbWDKeJxnDbYcUob5ZY5Lt5ZCMKuaGUvCJRrCtuugSMaqjowCGRempsDdJEt+cMaalhZ6gczklJB/IbdwENW9KeVFPoFNFzhxWUIS5ML9riVYhAtE6JE5jX0xiHNVIIPthb458cfA8daR0nYfYAUKogQArm0iBezOO+mPk5vCNWI+wwkyFCqNDXz/qxl1gAntuCJtSfq9OC3NkdhQlgYQ==` |
| staging | `genericServerPublicParams` | 225 | `AHILOIrFPXX9laLbalbA9+L1CXpSbM/bTJXZGZiuyK1JaI6dK5FHHWL6tWxmHKYAZTSYmElmJ5z2A5YcirjO/yfoemE03FItyaf8W1fE4p14hzb5qnrmfXUSiAIVrhaXVwIwSzH6RL/+EO8jFIjJ/YfExfJ8aBl48CKHgu1+A6kWynhttonvWWx6h7924mIzW0Czj2ROuh4LwQyZypex4GuOPW8sgIT21KNZaafgg+KbV7XM1x1tF3XA17B4uGUaDbDw2O+nR1+U5p6qHPzmJ7ggFjSN6Utu+35dS1sS0P9N` |
| staging | `backupServerPublicParams` | 225 | `AHYrGb9IfugAAJiPKp+mdXUx+OL9zBolPYHYQz6GI1gWjpEu5me3zVNSvmYY4zWboZHif+HG1sDHSuvwFd0QszSwuSF4X4kRP3fJREdTZ5MCR0n55zUppTwfHRW2S4sdQ0JGz7YDQIJCufYSKh0pGNEHL6hv79Agrdnr4momr3oXdnkpVBIp3HWAQ6IbXQVSG18X36GaicI1vdT0UFmTwU2KTneluC2eyL9c5ff8PcmiS+YcLzh0OKYQXB5ZfQ06d6DiINvDQLy75zcfUOniLAj0lGJiHxGczin/RXisKSR8` |
| production | `zkGroupServerPublicParams` | 673 | `AMhf5ywVwITZMsff/eCyudZx9JDmkkkbV6PInzG4p8x3VqVJSFiMvnvlEKWuRob/1eaIetR31IYeAbm0NdOuHH8Qi+Rexi1wLlpzIo1gstHWBfZzy1+qHRV5A4TqPp15YzBPm0WSggW6PbSn+F4lf57VCnHF7p8SvzAA2ZZJPYJURt8X7bbg+H3i+PEjH9DXItNEqs2sNcug37xZQDLm7X36nOoGPs54XsEGzPdEV+itQNGUFEjY6X9Uv+Acuks7NpyGvCoKxGwgKgE5XyJ+nNKlyHHOLb6N1NuHyBrZrgtY/JYJHRooo5CEqYKBqdFnmbTVGEkCvJKxLnjwKWf+fEPoWeQFj5ObDjcKMZf2Jm2Ae69x+ikU5gBXsRmoF94GXTLfN0/vLt98KDPnxwAQL9j5V1jGOY8jQl6MLxEs56cwXN0dqCnImzVH3TZT1cJ8SW1BRX6qIVxEzjsSGx3yxF3suAilPMqGRp4ffyopjMD1JXiKR2RwLKzizUe5e8XyGOy9fplzhw3jVzTRyUZTRSZKkMLWcQ/gv0E4aONNqs4P+NameAZYOD12qRkxosQQP5uux6B2nRyZ7sAV54DgFyLiRcq1FvwKw2EPQdk4HDoePrO/RNUbyNddnM/mMgj4FW65xCoT1LmjrIjsv/Ggdlx46ueczhMgtBunx1/w8k8V+l8LVZ8gAT6wkU5J+DPQalQguMg12Jzug3q4TbdHiGCmD9EunCwOmsLuLJkz6EcSYXtrlDEnAM+hicw7iergYLLlMXpfTdGxJCWJmP4zqUFeTTmsmhsjGBt7NiEB/9pFFEB3pSbf4iiUukw63Eo8Aqnf4iwob6X1QviCWuc8t0LUlT9vALgh/f2DPVOOmR0RW6bgRvc7DSF20V/omg+YBw==` |
| production | `genericServerPublicParams` | 225 | `AByD873dTilmOSG0TjKrvpeaKEsUmIO8Vx9BeMmftwUs9v7ikPwM8P3OHyT0+X3EUMZrSe9VUp26Wai51Q9I8mdk0hX/yo7CeFGJyzoOqn8e/i4Ygbn5HoAyXJx5eXfIbqpc0bIxzju4H/HOQeOpt6h742qii5u/cbwOhFZCsMIbElZTaeU+BWMBQiZHIGHT5IE0qCordQKZ5iPZom0HeFa8Yq0ShuEyAl0WINBiY6xE3H/9WnvzXBbMuuk//eRxXgzO8ieCeK8FwQNxbfXqZm6Ro1cMhCOF3u7xoX83QhpN` |
| production | `backupServerPublicParams` | 225 | `AJwNSU55fsFCbgaxGRD11wO1juAs8Yr5GF8FPlGzzvdJJIKH5/4CC7ZJSOe3yL2vturVaRU2Cx0n751Vt8wkj1bozK3CBV1UokxV09GWf+hdVImLGjXGYLLhnI1J2TWEe7iWHyb553EEnRb5oxr9n3lUbNAJuRmFM7hrr0Al0F0wrDD4S8lo2mGaXe0MJCOM166F8oYRQqpFeEHfiLnxA1O8ZLh7vMdv4g9jI5phpRBTsJ5IjiJrWeP0zdIGHEssUeprDZ9OUJ14m0v61eYJMKsf59Bn+mAT2a7YfB+Don9O` |

## 8. Sealed sender

| Env | Root | base64 (33-byte key, type byte 0x05) | hex | Source |
|-----|------|------|-----|--------|
| staging | 1 | `BbqY1DzohE4NUZoVF+L18oUPrK3kILllLEJh2UnPSsEx` | `05ba98d43ce8844e0d519a1517e2f5f2850facade420b9652c4261d949cf4ac131` | E1 `StagingConfig.java` lines 28-29 |
| staging | 2 | `BYhU6tPjqP46KGZEzRs1OL4U39V5dlPJ/X09ha4rErkm` | `058854ead3e3a8fe3a286644cd1b3538be14dfd5797653c9fd7d3d85ae2b12b926` | lines 30-31 |
| production | 1 | `BXu6QIKVz5MA8gstzfOgRQGqyLqOwNKHL6INkv3IHWMF` | `057bba408295cf9300f20b2dcdf3a04501aac8ba8ec0d2872fa20d92fdc81d6305` | E1 `LiveConfig.java` lines 28-29 |
| production | 2 | `BUkY0I+9+oPgDCn4+Ac6Iu813yvqkDr/ga8DzLxFxuk6` | `054918d08fbdfa83e00c29f8f8073a22ef35df2bea903aff81af03ccbc45c6e93a` | lines 30-31 |

Both roots of the environment are accepted (E1 lines 82-89; the
certificate validator gets the list, `SignalDependencies.java` line
401). Nothing else in certificate validation depends on the environment.
libsignal's known-certificate table (ids 2 = staging, 3 = production) is
built into libsignal and the same in both. A certificate referenced by
id validates only if its root is one of the configured roots
(`sealed-sender.md` §4.2-4.3).

## 9. TLS trust

| Client stack | Hosts | Trusted roots | Staging vs production |
|--------------|-------|---------------|-----------------------|
| libsignal-net (BoringSSL) | chat (`grpc.chat…`), CDSI, (SVR-B) | the two Signal roots compiled into libsignal: RSA-4096 `signal.cer`, SHA-256 of DER `ddb0f92bb95c8d6fd202ea6e8cc5ccd182b544f8cd696f47d580659ddc9df65a`; Ed25519 `signal-ed25519.cer`, SHA-256 of DER `2642f1f90b389fed2558d7324ca26d70e750b06f87c595c5de955e2560821657` | **same** for every domain config in both environments (E3 `env.rs`: every `cert:` is `SIGNAL_ROOT_CERTIFICATES`; `certs.rs` lines 8-13). Both DER files are byte-identical to copies found in `libsignal_jni_amd64.so` (E4) |
| OkHttp (`whisper.store`, BKS, password `whisper`) | REST chat, provisioning socket, storage/groups, CDNs 0/2/3, SVR2 | `signal-messenger-ca` (SHA-256 of DER `ddb0f92bb95c8d6fd202ea6e8cc5ccd182b544f8cd696f47d580659ddc9df65a`, the same certificate as libsignal's RSA root) and `textsecure-gcm-ca` (SHA-256 `098fd5403f63fef89fba4cb4b98135ca2e73dc42c0ed03a377e8383f2d1fe9ea`, expired 2023-03-23) | **same**: both config classes use one `WhisperTrustStore` instance for every URL (E1 line 43; `WhisperTrustStore.java`). `whisper.store` SHA-256 `388a24e1ab8c205b6135615e59335097595176e6f66ba29fca007070530c6bdf`, source and jar identical |

There is no certificate pinning beyond these root sets, and no other
environment-specific TLS setting. Min TLS 1.3 for libsignal-net hosts;
TLS 1.2/1.3 for OkHttp (`transport.md` §3.3, §5). Which root each
production host actually chains to was **not observed** (no contact). A
client should trust both roots of the relevant set, exactly as for
staging.

## 10. Registration and captcha

signal-cli has **no environment-specific captcha setting**. The hint text
it prints is the production URL in both environments:

| Context | URL printed (both envs) | Source |
|---------|-------------------------|--------|
| `register` needs a captcha | `https://signalcaptchas.org/registration/generate.html` | E1 `CommandUtil.java` line 127; jar |
| send result needs a rate-limit challenge captcha | `https://signalcaptchas.org/challenge/generate.html` | E1 `SendMessageResultUtils.java` line 99; jar |

The man page adds the staging page:
`https://signalcaptchas.org/staging/registration/generate.html` (E1
`man/signal-cli.1.adoc` lines 118-119, 199-200, and 1127-1128 for
`submitRateLimitChallenge`). There is no staging challenge page
mentioned.

| Key | Staging | Production |
|-----|---------|------------|
| registration captcha page | `https://signalcaptchas.org/staging/registration/generate.html` | `https://signalcaptchas.org/registration/generate.html` |
| challenge captcha page | `https://signalcaptchas.org/staging/registration/generate.html` (man page, see `rate-limit-challenge.md` §3 caveat) | `https://signalcaptchas.org/challenge/generate.html` |

The captcha token handling (stripping `signalcaptcha://`) is the same in
both environments. Registration itself differs only in the REST base URL
(§2).

## 11. Other services

| Service | Status in signal-cli 0.14.7 | Source |
|---------|-----------------------------|--------|
| Key transparency | **not used**. libsignal carries per-environment keys (staging signing key `ac0de1fd7f33552bbeb6ebc12b9d4ea10bf5f025c45073d3fb5f5648955a749e`, VRF key `ec3a268237cf5c47115cf222405d5f90cc633ebe05caf82c0dd5acf9d341dadb`; production signing key `a3973067984382cfa89ec26d7cc176680aefe92b3d2eba85159dad0b8354b622`, VRF key `3849cf116c7bc9aef5f13f0c61a7c246e5bade4eb7e1c7b0efcacdd8c1e6a6ff`; auditor keys staging `1123b13ee32479ae6af5739e5d687b51559abf7684120511f68cde7a21a0e755`, `bd1e26a0fbdbfa923486ccc9296f4227db490b4add29f5507775171ea0fb7a4e`, `093ee42d95502b3e81f4e604179c82c149fffb96167642b9eb81b03d6e2dd636`, production `2d973608e909a09e12cbdbd21ad58775fd72fe1034a5a079f26541d5764ce17f`, `2f217a86cd2dbc95d46a84420942a95877b3723f634bc64bb9e406796df746ef`, `7fe5d91de235188486d8fb836a6da37e625e2b10eb6d144185b9364cc83cbbb6`), but no signal-cli class references key transparency | E3 `env.rs` lines 363-393; E4 byte search (`KeyTransparency`: no hit) |
| Remote config | **not fetched**. signal-cli gives libsignal an empty remote-config map in both environments | E1 `SignalDependencies.java` lines 166-172 |
| Payments (MobileCoin) | **not configured**. Only a profile payment-address field; no payment host | E4 byte search (`MobileCoin` only in profile classes) |
| SVR-B / backups | **not used** (§6) | |
| `signal.me`, `signal.art` | URL formats for username and sticker links, never contacted | E4 strings in `libsignal-cli-0.14.7.jar` |

A byte search of all signal-cli and signal-service-java jars for
`signal.org` host names finds only the 15 URLs in `LiveConfig` and
`StagingConfig` (E4). The service jars contain no host names at all.

## 12. Agreement with existing specs (staging values)

| Spec | Staging value stated | Result |
|------|----------------------|--------|
| `transport.md` | `grpc.chat.staging.signal.org:443`, `/v1/websocket/`, REST `https://chat.staging.signal.org`, static IPs, `whisper.store` hash `388a24e1…6bdf` and both libsignal roots | agrees. Its remark "Production uses `grpc.chat.signal.org`" agrees |
| `linking.md` | REST base URL with `wss://` + `/v1/websocket/provisioning/` | agrees |
| `cdsi.md` | `cdsi.staging.signal.org`, MRENCLAVE `6d9b9649…1c0a`, prefix `/cdsi-staging` | agrees |
| `svr.md` | `https://svr2.staging.signal.org`, enclaves `3c699f49…` then `97f151f6…`, Raft table incl. live rows | agrees, including the live rows and the live URL. Minor wording: svr.md writes the connection URL as `wss://…`; the code hands `https://…/v1/<mr>` to OkHttp, which upgrades it as `wss`. Same on the wire |
| `attestation.md` | §2.1 endpoints, §7.1/7.2 tables (staging), unused `CDSI_MRENCLAVE` | agrees. It lists only staging enclaves; the production values are in §5-6 above |
| `zkgroup-groups.md` §8.1 | 673/225/225-byte blobs, prefixes for both environments | agrees |
| `sealed-sender.md` §4.3 | four roots | agrees exactly |
| `attachments.md` | CDN 0/2/3 staging bases | agrees |
| `storage.md`, `groups.md` | `https://storage-staging.signal.org` (production `https://storage.signal.org`) | agrees |
| `registration.md`, `rate-limit-challenge.md` | staging captcha page from the man page | agrees |
| `signal-cli-data-dir.md` | `"LIVE"`/`"STAGING"`, null = any | agrees; add: an account file with no `serviceEnvironment` counts as `LIVE` (§1) |

No discrepancy was found.

## 13. Vector file `testdata/vectors/environments.json`

Top-level keys: `description`, `encodings`, `staging`, `production`,
`citations`. `staging` and `production` have the same keys:

| Key | Type | Meaning (section) |
|-----|------|-------------------|
| `account_environment` | string | value in account files and `accounts.json` (§1) |
| `cli_value` | string | `--service-environment` value (§1) |
| `libsignal_net_environment` | string | libsignal `Network.Environment` name (§1) |
| `chat_rest_url` | string | REST base URL (§2) |
| `chat_ws_host` | string | libsignal-net chat host, both sockets (§2) |
| `chat_grpc_host` | string | same value as `chat_ws_host` (§2) |
| `chat_ws_path` | string | (§2) |
| `provisioning_url` | string | (§2) |
| `storage_url` | string | storage and groups (§3) |
| `groups_url` | string | same as `storage_url` (§3) |
| `cdn_urls` | object | CDN number (string) → base URL (§4) |
| `cdsi_host`, `cdsi_url`, `cdsi_mrenclave`, `cdsi_advisories` | | (§5); `cdsi_url` is the full WebSocket URL |
| `signal_cli_cdsi_mrenclave_unused` | string | the unused config value (§5) |
| `svr2_url` | string | base URL (§6) |
| `svr2_enclaves` | array | in signal-cli's order; each has `role`, `mrenclave`, `url` (full connection URL as passed to OkHttp), `min_voting_replicas`, `max_voting_replicas`, `super_majority`, `group_id` (decimal **string**, since some exceed 2^63), `db_version`, `attestation_timeout`, `simulated`, `advisories` (§6) |
| `svrb` | object | `used_by_signal_cli` false, `host`, `enclaves` (§6) |
| `zkgroup_server_public_params`, `zkgroup_generic_server_public_params`, `zkgroup_backup_server_public_params` | base64 | (§7) |
| `sealed_sender_trust_roots` | array of base64 | in configured order (§8) |
| `tls_roots` | object | `libsignal_net` and `okhttp_whisper_store`, each `{hosts, sha256_der}` (§9) |
| `captcha_registration_url`, `captcha_challenge_url` | string | (§10) |
| `keytrans` | object | `used_by_signal_cli` false, `signing_key`, `vrf_key`, `auditor_keys` (hex) (§11) |
| `fronting` | object | `used_by_signal_cli` false, reflector hosts and path prefixes (§2) |
| `static_ip_fallback` | object | host → list of IPs (§2) |

`citations` maps each key to its source (ref, file, lines).
