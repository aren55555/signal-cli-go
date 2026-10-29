		echo "B1 fault injected on this device (its copy of the sender key was dropped); it cannot match B2 by design" >"$RUN/$pre-1send/skip-B1"
#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Driver of the live group interop runs (spec/testing/groups-interop.md).
# Staging only. Run from /work inside the sandbox:
#
#   tools/interop/run.sh STEP [ARGS]
#
# Live data goes to .data/interop/ (gitignored). Numbers come from the
# environment (A, B) and /tmp/staging-number-c or SIGNAL_STAGING_NUMBER_C,
# and are masked in everything printed.
set -u
cd /work

A=$SIGNAL_STAGING_NUMBER_A
B=$SIGNAL_STAGING_NUMBER_B
C=${SIGNAL_STAGING_NUMBER_C:-$(tr -d ' \n\r' < /tmp/staging-number-c)}
OURS=$HOME/.cache/sgo
TOOL=$HOME/.cache/interop
ROOT=.data/interop
RUN=${INTEROP_RUN:-$ROOT/run-$(date -u +%Y%m%d)}
mkdir -p "$RUN" "$HOME/.cache/jvmtmp"

mask() { sed -e "s/${A//+/\\+}/<A>/g" -e "s/${B//+/\\+}/<B>/g" -e "s/${C//+/\\+}/<C>/g"; }
log() { echo "[$(date -u +%H:%M:%S)] $*" | mask | tee -a "$RUN/run.log" >&2; }
result() { echo "$1 $2" | tee -a "$RUN/results.txt" >&2; }

# Devices (§2): kind, data directory, number.
dev() {
	case $1 in
	A1) echo "ref .data/capture-a $A" ;;
	A2) echo "ours .data/cli-a $A" ;;
	B1) echo "ours .data/cli-b $B" ;;
	B2) echo "ref $ROOT/ref-b $B" ;;
	C1) echo "ref .data/capture-c $C" ;;
	C2) echo "ours $ROOT/cli-c $C" ;;
	A3) echo "ours ${A3DIR:-$ROOT/dev-add} $A" ;; # scratch device of S4 device add
	*) echo "unknown device $1" >&2; exit 2 ;;
	esac
}
num() { case $1 in A) echo "$A" ;; B) echo "$B" ;; C) echo "$C" ;; esac; }

# quote turns arguments into one shell-quoted string: `just oracle` joins
# its arguments with spaces and hands them to a shell.
quote() {
	local out="" a
	for a in "$@"; do out+="'${a//\'/\'\\\'\'}' "; done
	printf '%s' "$out"
}

# raw DEV OUT ERR ARGS...: one command on a device, stdout and stderr to
# files. Our client always runs with -v (the path lines need it).
raw() {
	local d=$1 out=$2 err=$3; shift 3
	local kind dir n
	read -r kind dir n <<<"$(dev "$d")"
	if [ "$kind" = ours ]; then
		"$OURS" -v --service-environment staging -d "$dir" -a "$n" "$@" >"$out" 2>"$err"
	else
		LANG=C.UTF-8 just oracle "$(quote --service-environment staging -c "$dir" -a "$n" "$@")" >"$out" 2>"$err"
	fi
}

# on DEV TAG ARGS...: raw with pacing and the §7 backoff. Output files are
# $RUN/cmd/TAG.{out,err}. Exits 3 on a captcha challenge.
BACKOFF=(60 120 240 480)
on() {
	local d=$1 tag=$2; shift 2
	mkdir -p "$RUN/cmd"
	local out=$RUN/cmd/$tag.out err=$RUN/cmd/$tag.err i rc
	for i in 0 1 2 3 4; do
		raw "$d" "$out" "$err" "$@"
		rc=$?
		if command grep -qiE 'captcha|proof.?required|\b428\b' "$err" "$out"; then
			log "$d $tag: staging asks for a captcha challenge; stopping (ask the operator)"
			mask <"$err" | tail -5 >&2
			exit 3
		fi
		if [ $rc -ne 0 ] && command grep -qiE '\b(429|413)\b|rate.?limit' "$err" "$out"; then
			[ $i -eq 4 ] && { log "$d $tag: still rate-limited after 4 back-offs"; result RATE-LIMITED "$tag"; exit 4; }
			local wait=${BACKOFF[$i]}
			local ra; ra=$(command grep -oiE 'retry.?after[^0-9]*[0-9]+' "$err" | command grep -oE '[0-9]+$' | head -1)
			[ -n "$ra" ] && wait=$((ra + 5))
			log "$d $tag: rate limited, waiting ${wait}s"
			sleep "$wait"
			continue
		fi
		sleep 5 # pacing: at least 5 s between staging commands
		return $rc
	done
}

# ts TAG: the timestamp a send printed (last number-only line).
ts() { command grep -E '^[0-9]{13}$' "$RUN/cmd/$1.out" | tail -1; }

# recv SCN DEV FMT: snapshot, then receive on DEV into the scenario
# capture (§6).
recv() {
	local scn=$1 d=$2 fmt=$3
	local kind dir n
	read -r kind dir n <<<"$(dev "$d")"
	local sd=$RUN/$scn
	mkdir -p "$sd/pre" "$sd/out" "$sd/logs" "$sd/envelopes"
	rm -rf "$sd/pre/$d" && cp -a "$dir" "$sd/pre/$d"
	local o=plain-text ext=txt
	[ "$fmt" = json ] && o=json ext=json
	export SIGNAL_CLI_GO_CAPTURE_DIR=$PWD/$sd/envelopes/$d
	on "$d" "$scn-recv-$d" -o "$o" receive -t 10
	unset SIGNAL_CLI_GO_CAPTURE_DIR
	cp "$RUN/cmd/$scn-recv-$d.out" "$sd/out/$d.$ext"
	cp "$RUN/cmd/$scn-recv-$d.err" "$sd/logs/$d.log"
}

# Assertions (§3). Each prints PASS/FAIL lines into results.txt.
check_send_path() { # SCN TAG KIND(token|story)
	local scn=$1 err=$RUN/cmd/$2.err kind=$3 want
	[ "$kind" = story ] && want="multi-recipient endpoint with story" || want="multi-recipient endpoint with group-send token"
	if command grep -q "Group send path: $want" "$err" && command grep -q "multi-recipient endpoint accepted" "$err" &&
		! command grep -qE "Group send path: (1:1 fallback|story not sent)" "$err"; then
		result PASS "$scn send-path ($want)"
	else
		result FAIL "$scn send-path: $(command grep -o 'Group send path: .*' "$err" | tr '\n' ';' | mask)"
	fi
}
check_recv_path() { # SCN DEV TS: our receiver saw the message via multi-recipient
	local scn=$1 d=$2 t=$3 p
	p=$("$TOOL" paths "$RUN/$scn/logs/$d.log" "$t")
	if echo "$p" | command grep -q "Envelope path: multi-recipient (sealed sender v2, sender key message"; then
		result PASS "$scn recv-path $d"
	else
		result FAIL "$scn recv-path $d: ${p:-no envelope}"
	fi
}
check_same() { # SCN DEVREF DEVOURS FMT TS: byte-identical after §5 normalization
	local scn=$1 r=$2 o=$3 fmt=$4 t=$5 ext=txt f=plain
	[ "$fmt" = json ] && { ext=json; f=json; }
	local sd=$RUN/$scn
	mkdir -p "$sd/diff"
	if "$TOOL" block "$sd/out/$r.$ext" "$f" "$t" >"$sd/diff/$r.$ext" 2>&1 &&
		"$TOOL" block "$sd/out/$o.$ext" "$f" "$t" >"$sd/diff/$o.$ext" 2>&1 &&
		cmp -s "$sd/diff/$r.$ext" "$sd/diff/$o.$ext"; then
		result PASS "$scn render $r=$o ($fmt)"
	else
		diff "$sd/diff/$r.$ext" "$sd/diff/$o.$ext" >"$sd/diff/$r-$o.$ext.diff"
		result FAIL "$scn render $r!=$o ($fmt), see $sd/diff/$r-$o.$ext.diff"
	fi
}
# check_count SCN DEV TS PATTERN N: our receiver logged exactly N matching
# envelope-path lines for timestamp TS (SKDM counts use the SKDM's own ts).
check_skdm() { # SCN DEV N: N sender-key distribution envelopes (1:1, no group id)
	local scn=$1 d=$2 want=$3 got
	got=$(command grep -c "Envelope path: 1:1 (sealed sender v1, \(prekey\|whisper\) message, group id false)" "$RUN/$scn/logs/$d.log")
	[ "$got" = "$want" ] && result PASS "$scn skdm-count $d=$want" || result FAIL "$scn skdm-count $d=$got want $want"
}

# Directions (§2).
direction() {
	case $1 in
	ours) SENDER=B1 SYNC=B2 RECV="A1 A2 C1 C2" PAIRS="A1:A2 C1:C2" GROUP=G-main OURS_RECV="A2 C2" ;;
	ref) SENDER=C1 SYNC=C2 RECV="A1 A2 B1 B2" PAIRS="A1:A2 B2:B1" GROUP=G-ref OURS_RECV="A2 B1" ;;
	linked) SENDER=A2 SYNC=A1 RECV="B1 B2 C1 C2" PAIRS="B2:B1 C1:C2" GROUP=G-main OURS_RECV="B1 C2" ;;
	*) echo "direction ours|ref|linked" >&2; exit 2 ;;
	esac
	read -r SKIND _ _ <<<"$(dev "$SENDER")"
}

groupid() { # DEV TITLE (looked up in an our-client store)
	local kind dir n
	read -r kind dir n <<<"$(dev "$1")"
	"$TOOL" group-id "$dir" "$n" "$2"
}

receive_all() { # SCN FMT DEVS...
	local scn=$1 fmt=$2 d; shift 2
	for d in "$@"; do recv "$scn" "$d" "$fmt"; done
}

# send_and_check SCN KIND(token|story) FMT ARGS...: the common shape of S1,
# S2 and S6: send on $SENDER, receive everywhere, assert path and rendering.
send_and_check() {
	local scn=$1 kind=$2 fmt=$3; shift 3
	log "$scn: $SENDER sends ($*)"
	on "$SENDER" "$scn-send" "$@" || { result FAIL "$scn send failed: $(tail -1 "$RUN/cmd/$scn-send.err" | mask)"; return 1; }
	local t; t=$(ts "$scn-send")
	[ -z "$t" ] && { result FAIL "$scn no timestamp"; return 1; }
	mkdir -p "$RUN/$scn" && echo "$t" >"$RUN/$scn/ts"
	[ "$SKIND" = ours ] && check_send_path "$scn" "$scn-send" "$kind"
	receive_all "$scn" "$fmt" $RECV $SYNC
	local d p
	for d in $OURS_RECV; do check_recv_path "$scn" "$d" "$t"; done
	if [ "$kind" != typing ]; then
		for p in $PAIRS; do check_same "$scn" "${p%%:*}" "${p##*:}" "$fmt" "$t"; done
	fi
}

# ---- S3-S5 (plan §4) ------------------------------------------------------

LAST_TS=""
# gsend SCN ARGS...: a send on $SENDER; LAST_TS gets its timestamp.
gsend() {
	local scn=$1; shift
	log "$scn: $SENDER: $*"
	on "$SENDER" "$scn-send" "$@" || { result FAIL "$scn send failed: $(tail -1 "$RUN/cmd/$scn-send.err" | mask)"; LAST_TS=""; return 1; }
	LAST_TS=$(ts "$scn-send")
	mkdir -p "$RUN/$scn" && echo "$LAST_TS" >"$RUN/$scn/ts"
}
senderlog() { command grep -o 'Group send path: .*' "$RUN/cmd/$1-send.err" | sed 's/, distribution [0-9a-f-]*//'; }
# expect_ours SCN WANT...: our sender's path lines contain every WANT
# (fixed strings, or !STRING for must-not-contain).
expect_ours() {
	local scn=$1 w ok=1 lines; shift
	lines=$(senderlog "$scn")
	for w in "$@"; do
		if [ "${w:0:1}" = "!" ]; then echo "$lines" | command grep -qF -- "${w:1}" && ok=0
		else echo "$lines" | command grep -qF -- "$w" || ok=0; fi
	done
	[ $ok = 1 ] && result PASS "$scn sender log: $*" || result FAIL "$scn sender log: want [$*] got [$(echo "$lines" | tr '\n' ';' | mask)]"
}
# expect_recv SCN DEV WANT: our receiver's envelope path for LAST_TS
# starts with WANT ("multi-recipient", "1:1" or "none").
expect_recv() {
	local scn=$1 d=$2 want=$3 p
	p=$("$TOOL" paths "$RUN/$scn/logs/$d.log" "$LAST_TS" | head -1)
	case $want in
	none) [ -z "$p" ] && result PASS "$scn $d got nothing" || result FAIL "$scn $d should get nothing: $p" ;;
	*) echo "$p" | command grep -q "^Envelope path: $want " && result PASS "$scn recv-path $d $want" || result FAIL "$scn recv-path $d want $want: ${p:-no envelope}" ;;
	esac
}
# skdm SCN DEV SENDER_ACI: SKDM envelopes our receiver got from the sender
# in this step (1:1 with a group id, other timestamps than LAST_TS).
skdm() {
	command grep -o "Envelope path: 1:1 (sealed sender v1, [a-z]* message, group id true) from $3\.[0-9]*, timestamp [0-9]*" "$RUN/$1/logs/$2.log" |
		command grep -vc "timestamp $LAST_TS\$"
}
expect_skdm() { # SCN DEV SENDER_ACI N
	local n; n=$(skdm "$1" "$2" "$3")
	[ "$n" = "$4" ] && result PASS "$1 skdm $2=$4" || result FAIL "$1 skdm $2=$n want $4"
}
aci_of() { local kind dir n; read -r kind dir n <<<"$(dev "$1")"; "$TOOL" aci "$dir" "$n"; }
render_pairs() { local scn=$1 p; shift; for p in "$@"; do check_same "$scn" "${p%%:*}" "${p##*:}" plain "$LAST_TS"; done; }

# S3: endorsements (ours direction only; the reference cannot be edited,
# its behaviour is covered by s1-ref and its first attempt).
s3() {
	direction ours
	local g; g=$(groupid B1 G-main)
	read -r kind dir n <<<"$(dev B1)"
	# 1. refresh: expiry within 2 hours forces a fetch before the send.
	"$TOOL" expire-endorsements "$dir" "$n" "$g" 1h
	gsend s3-refresh send -g "$g" -m "S3 refresh: endorsements expiring" || return
	local exp; read -r exp _ <<<"$("$TOOL" endorsement-expiry "$dir" "$n" "$g")"
	[ "$exp" -gt $(( $(date +%s%3N) + 2*3600*1000 )) ] && result PASS "s3-refresh endorsements refreshed" || result FAIL "s3-refresh endorsements not refreshed ($exp)"
	expect_ours s3-refresh "multi-recipient endpoint with group-send token" "2 recipient(s)" "accepted" "!1:1 fallback"
	receive_all s3-refresh plain A1 A2 C1 C2 B2
	expect_recv s3-refresh A2 multi-recipient; expect_recv s3-refresh C2 multi-recipient
	render_pairs s3-refresh A1:A2 C1:C2
	sleep 55
	# 2. coverage: the combined token covers exactly the two targets; a
	# wrong combination would answer 401 (stories.md §3.4).
	gsend s3-token send -g "$g" -m "S3 token coverage" || return
	expect_ours s3-token "group-send token" "2 recipient(s), 4 device(s)" "accepted (0 unregistered)" "!answered"
	receive_all s3-token plain A1 A2 C1 C2 B2
	expect_recv s3-token A2 multi-recipient; expect_recv s3-token C2 multi-recipient
	render_pairs s3-token A1:A2 C1:C2
	sleep 55
	# 4. control: drop C's endorsement; one member left qualifies.
	"$TOOL" drop-endorsement "$dir" "$n" "$g" "$(aci_of C2)"
	gsend s3-control send -g "$g" -m "S3 control: expected fallback" || return
	expect_ours s3-control "1:1 fallback for 2 of 2 recipient(s): only 1 member(s) qualify" "!multi-recipient endpoint"
	receive_all s3-control plain A1 A2 C1 C2 B2
	expect_recv s3-control A2 1:1; expect_recv s3-control C2 1:1
	render_pairs s3-control A1:A2 C1:C2
	"$TOOL" expire-endorsements "$dir" "$n" "$g" 1h # the next send refetches them
}

# S4: redistribution. DIRECTION ours (sender B1, joiner C) or ref (sender
# C1, joiner B).
s4() {
	direction "$1"
	local dir=$1 J1 J2 JO SENDER_RECV pre=s4-$1
	if [ "$dir" = ours ]; then J1=C1 J2=C2 JO=C2 JP=C1:C2; else J1=B2 J2=B1 JO=B1 JP=B2:B1; fi
	local saci aciA aciJ
	if [ "$SKIND" = ours ]; then saci=$(aci_of "$SENDER"); else saci=$(aci_of "$SYNC"); fi
	aciA=$(aci_of A2); aciJ=$(aci_of "$JO")
	local all="A1 A2 $J1 $J2 $SYNC" members="A1 A2 $SYNC"
	# create with A only, invite link on
	gsend "$pre-1create" updateGroup -n "G-churn-$dir" -m "$aciA" --link enabled || return
	local g; g=$(command grep -o 'Created new group: "[^"]*"' "$RUN/cmd/$pre-1create-send.out" | cut -d'"' -f2)
	[ -z "$g" ] && { result FAIL "$pre create: no group id"; return; }
	echo "$g" >"$RUN/$pre-group"
	receive_all "$pre-1create" plain $members
	on "$SENDER" "$pre-link" listGroups -d -g "$g"
	local link; link=$(command grep -o 'https://signal.group/#[^ ]*' "$RUN/cmd/$pre-link.out" | head -1)
	[ -z "$link" ] && { result FAIL "$pre no invite link"; return; }
	sleep 30
	# control: one other member
	gsend "$pre-2control" send -g "$g" -m "S4 $dir control: only A" || return
	[ "$SKIND" = ours ] && expect_ours "$pre-2control" "1:1 fallback" "only 1 member(s) qualify"
	receive_all "$pre-2control" plain $members
	expect_recv "$pre-2control" A2 1:1
	sleep 30
	# join, then the sender commits a change (fresh endorsements)
	on "$JO" "$pre-join" joinGroup --uri "$link" || result FAIL "$pre join failed: $(tail -1 "$RUN/cmd/$pre-join.err" | mask)"
	receive_all "$pre-3joined" plain "$SENDER" A1 A2 "$J1" $SYNC
	gsend "$pre-4refresh" updateGroup -g "$g" -d "after join" || return
	[ "$SKIND" = ours ] && expect_ours "$pre-4refresh" "sender key distribution to 2 recipient(s)" "new key true" "multi-recipient endpoint with group-send token" "accepted" "!1:1 fallback"
	receive_all "$pre-4refresh" plain $all
	expect_recv "$pre-4refresh" A2 multi-recipient; expect_recv "$pre-4refresh" "$JO" multi-recipient
	[ "$SKIND" = ref ] && { expect_skdm "$pre-4refresh" "$JO" "$saci" 1; expect_skdm "$pre-4refresh" A2 "$saci" 1; }
	sleep 30
	gsend "$pre-5joinmsg" send -g "$g" -m "S4 $dir after join" || return
	[ "$SKIND" = ours ] && expect_ours "$pre-5joinmsg" "multi-recipient endpoint with group-send token" "2 recipient(s)" "accepted" "!sender key distribution" "!1:1 fallback"
	receive_all "$pre-5joinmsg" plain $all
	expect_recv "$pre-5joinmsg" A2 multi-recipient; expect_recv "$pre-5joinmsg" "$JO" multi-recipient
	[ "$SKIND" = ref ] && expect_skdm "$pre-5joinmsg" "$JO" "$saci" 0
	render_pairs "$pre-5joinmsg" A1:A2 "$JP"
	sleep 30
	# leave, control, rejoin
	on "$JO" "$pre-quit" quitGroup -g "$g" || result FAIL "$pre quit failed"
	receive_all "$pre-6left" plain "$SENDER" A1 A2 "$J1" $SYNC
	gsend "$pre-7leftmsg" send -g "$g" -m "S4 $dir while left" || return
	[ "$SKIND" = ours ] && expect_ours "$pre-7leftmsg" "1:1 fallback" "only 1 member(s) qualify"
	receive_all "$pre-7leftmsg" plain $all
	expect_recv "$pre-7leftmsg" "$JO" none
	on "$JO" "$pre-rejoin" joinGroup --uri "$link" || result FAIL "$pre rejoin failed: $(tail -1 "$RUN/cmd/$pre-rejoin.err" | mask)"
	receive_all "$pre-8rejoined" plain "$SENDER" A1 A2 "$J1" $SYNC
	gsend "$pre-9refresh" updateGroup -g "$g" -d "after rejoin" || return
	# no rotation on leave: the rejoiner still holds the key (stories.md §3.7)
	[ "$SKIND" = ours ] && expect_ours "$pre-9refresh" "multi-recipient endpoint with group-send token" "2 recipient(s)" "accepted" "!sender key distribution" "!1:1 fallback"
	receive_all "$pre-9refresh" plain $all
	expect_recv "$pre-9refresh" "$JO" multi-recipient
	[ "$SKIND" = ref ] && expect_skdm "$pre-9refresh" "$JO" "$saci" 0
	sleep 30
	gsend "$pre-10rejoinmsg" send -g "$g" -m "S4 $dir after rejoin" || return
	[ "$SKIND" = ours ] && expect_ours "$pre-10rejoinmsg" "multi-recipient endpoint with group-send token" "accepted" "!sender key distribution" "!1:1 fallback"
	receive_all "$pre-10rejoinmsg" plain $all
	expect_recv "$pre-10rejoinmsg" "$JO" multi-recipient
	render_pairs "$pre-10rejoinmsg" A1:A2 "$JP"
	sleep 30
	# removal, control, re-add (the sender's own commits refresh endorsements)
	gsend "$pre-11remove" updateGroup -g "$g" -r "$aciJ" || return
	receive_all "$pre-11remove" plain $all
	gsend "$pre-12removedmsg" send -g "$g" -m "S4 $dir while removed" || return
	[ "$SKIND" = ours ] && expect_ours "$pre-12removedmsg" "1:1 fallback" "only 1 member(s) qualify"
	receive_all "$pre-12removedmsg" plain $all
	expect_recv "$pre-12removedmsg" "$JO" none
	gsend "$pre-13readd" updateGroup -g "$g" -m "$aciJ" || return
	[ "$SKIND" = ours ] && expect_ours "$pre-13readd" "multi-recipient endpoint with group-send token" "accepted" "!sender key distribution" "!1:1 fallback"
	receive_all "$pre-13readd" plain $all
	expect_recv "$pre-13readd" "$JO" multi-recipient
	[ "$SKIND" = ref ] && expect_skdm "$pre-13readd" "$JO" "$saci" 0
	sleep 30
	gsend "$pre-14readdmsg" send -g "$g" -m "S4 $dir after re-add" || return
	[ "$SKIND" = ours ] && expect_ours "$pre-14readdmsg" "multi-recipient endpoint with group-send token" "accepted" "!sender key distribution" "!1:1 fallback"
	receive_all "$pre-14readdmsg" plain $all
	expect_recv "$pre-14readdmsg" "$JO" multi-recipient
	render_pairs "$pre-14readdmsg" A1:A2 "$JP"
}

# S4 device add: A links a scratch signal-cli-go device (A3); the sender's
# next send must reach it. DIRECTION ours|ref; group G-churn-DIRECTION.
s4dev() {
	direction "$1"
	local dir=$1 pre=s4-$1-15devadd g; g=$(cat "$RUN/s4-$1-group")
	local JP; [ "$dir" = ours ] && JP=C1:C2 || JP=B2:B1
	local saci; [ "$SKIND" = ours ] && saci=$(aci_of "$SENDER") || saci=$(aci_of "$SYNC")
	local a3=$ROOT/dev-add-$dir
	rm -rf "$a3" && mkdir -p "$a3"
	( timeout 300 "$OURS" --service-environment staging -d "$a3" link -n "dev-add-$dir" >"$RUN/cmd/$pre-link.out" 2>"$RUN/cmd/$pre-link.err" & )
	local uri i
	for i in $(seq 1 30); do uri=$(command grep -o 'sgnl://[^ ]*' "$RUN/cmd/$pre-link.out" | head -1); [ -n "$uri" ] && break; sleep 1; done
	on A1 "$pre-adddevice" addDevice --uri "$uri" || { result FAIL "$pre addDevice failed"; return; }
	sleep 20
	command grep -q "Associated with" "$RUN/cmd/$pre-link.out" || { result FAIL "$pre link did not finish"; return; }
	A3DIR=$a3
	on A3 "$pre-a3init" receive -t 10 >/dev/null
	sleep 30
	gsend "$pre" send -g "$g" -m "S4 $dir after A added a device" || return
	[ "$SKIND" = ours ] && expect_ours "$pre" "answered 409 (device mismatch), retrying" "sender key distribution to 1 recipient(s)" "attempt 2" "accepted" "!1:1 fallback"
	receive_all "$pre" plain A1 A2 A3 "${JP%%:*}" "${JP##*:}" $SYNC
	expect_recv "$pre" A3 multi-recipient; expect_recv "$pre" A2 multi-recipient
	[ "$SKIND" = ref ] && { expect_skdm "$pre" A3 "$saci" 1; expect_skdm "$pre" A2 "$saci" 1; } # an SKDM is a 1:1 message to the account: every device of A gets it
	render_pairs "$pre" A1:A2 A1:A3
	# clean up the scratch device
	on A1 "$pre-devices" listDevices
	local id; id=$(command grep -B1 "Name: dev-add-$dir" "$RUN/cmd/$pre-devices.out" | command grep -o 'Device [0-9]*' | command grep -o '[0-9]*$')
	[ -n "$id" ] && on A1 "$pre-remove" removeDevice -d "$id" && log "$pre: removed scratch device $id"
}

# S5: retry receipts. ref: our B1 forgets C1's sender key, C1 sends in
# G-ref. ours: our B1 starts a new sender key for G-main and pretends C
# already has it, then sends.
s5() {
	direction "$1"
	local pre=s5-$1 g
	if [ "$1" = ref ]; then
		g=$(groupid B1 "${S5_GROUP:-G-ref}")
		local kind dir n; read -r kind dir n <<<"$(dev B1)"
		local caci; caci=$(aci_of C2)
		# Prime: B1 takes C1's current key, so C1 marks it shared and the
		# test send carries no SKDM for B1 (an earlier session renewal would
		# otherwise make C1 hand its key out again).
		gsend "$pre-0prime" send -g "$g" -m "S5 ref: priming" || return
		receive_all "$pre-0prime" plain B1 B2 A1 A2 C2
		sleep 20
		"$TOOL" drop-sender-keys "$dir" "$n" "$caci" || log "$pre: B1 holds no sender key from C (already dropped)"
		gsend "$pre-1send" send -g "$g" -m "S5 ref: retry receipt test" || return
		receive_all "$pre-1send" plain B1 B2 A1 A2 C2
		expect_recv "$pre-1send" B1 multi-recipient
		receive_all "$pre-2retry" plain C1        # the reference handles the retry receipt and resends
		receive_all "$pre-3resent" plain B1
		expect_recv "$pre-3resent" B1 1:1
		LAST_TS=$(cat "$RUN/$pre-1send/ts")
		local blk; blk=$("$TOOL" block "$RUN/$pre-3resent/out/B1.txt" plain "$LAST_TS" 2>/dev/null)
		[ -n "$blk" ] && result PASS "$pre B1 rendered the resent message" || result FAIL "$pre B1 never rendered the message"
		mkdir -p "$RUN/$pre-3resent/out"; cp "$RUN/$pre-1send/out/B2.txt" "$RUN/$pre-3resent/out/B2.txt"
		check_same "$pre-3resent" B2 B1 plain "$LAST_TS"
		render_pairs "$pre-1send" A1:A2
	else
		g=$(groupid B1 G-main)
		local kind dir n; read -r kind dir n <<<"$(dev B1)"
		"$TOOL" mark-shared "$dir" "$n" "$g" "$(aci_of C2)" || { result FAIL "$pre: mark-shared failed"; return; }
		gsend "$pre-1send" send -g "$g" -m "S5 ours: retry receipt test" || return
		expect_ours "$pre-1send" "sender key distribution to 1 recipient(s)" "new key true" "multi-recipient endpoint with group-send token" "accepted"
		receive_all "$pre-1send" plain A1 A2 C1 C2 B2
		expect_recv "$pre-1send" C2 multi-recipient
		render_pairs "$pre-1send" A1:A2
		receive_all "$pre-2retry" plain B1        # our sender handles both retry receipts
		command grep -c "resend after a retry receipt" "$RUN/$pre-2retry/logs/B1.log" | { read -r c; [ "$c" -ge 1 ] && result PASS "$pre B1 resent ($c)" || result FAIL "$pre B1 did not resend"; }
		receive_all "$pre-3resent" plain C1 C2
		LAST_TS=$(cat "$RUN/$pre-1send/ts")
		expect_recv "$pre-3resent" C2 1:1
		check_same "$pre-3resent" C1 C2 plain "$LAST_TS"
	fi
}

step=${1:-}
shift || true
case $step in
build)
	export GOTMPDIR=$HOME/.cache/gotmp GOFLAGS=-mod=vendor CGO_ENABLED=0
	go build -o "$OURS" ./cmd/signal-cli-go && go build -o "$TOOL" ./tools/interop
	;;
on) # on DEV TAG ARGS... (one ad-hoc command, output masked)
	d=$1 tag=$2; shift 2
	on "$d" "$tag" "$@"; rc=$?
	mask <"$RUN/cmd/$tag.out"; mask <"$RUN/cmd/$tag.err" | command grep -vE '^JAVA_OPTS=' | tail -20
	exit $rc
	;;
receive) # receive SCN FMT DEVS...
	receive_all "$@"
	;;
s3) s3 ;;
s4) s4 "$1" ;;
s4dev) s4dev "$1" ;;
s5) s5 "$1" ;;
s1 | s2 | s6) # sN DIRECTION FMT
	direction "$1"
	fmt=${2:-plain}
	scn=$step-$1-$fmt
	g=$(groupid "$( [ "$SKIND" = ours ] && echo "$SENDER" || echo "${OURS_RECV%% *}")" "$GROUP")
	case $step in
	s1) send_and_check "$scn" token "$fmt" send -g "$g" -m "S1 $1 $fmt: group message $(date -u +%H%M%S)" -a tools/interop/testdata/pic.png ;;
	s2) send_and_check "$scn" story "$fmt" sendStory -g "$g" -a tools/interop/testdata/pic.png ;;
	s6) send_and_check "$scn" typing "$fmt" sendTyping -g "$g" ;;
	esac
	;;
*)
	sed -n '2,12p' "$0"
	echo "steps: build | on DEV TAG ARGS... | receive SCN FMT DEVS... | s1|s2|s6 DIRECTION FMT | s3 | s4 DIR | s4dev DIR | s5 DIR" >&2
	exit 2
	;;
esac
