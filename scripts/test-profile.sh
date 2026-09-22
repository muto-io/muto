#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Profile the Ginkgo integration suites and rank the slowest specs.
#
# For each suite the script builds the test binary (timed separately), runs it
# with a Ginkgo JSON report, records the test process' CPU time and peak RSS
# (GNU time), and samples CPU/memory of the k3s/kind cluster containers
# (docker stats). It then writes a Markdown report with the slowest nodes,
# time per file, and CPU/memory metrics.
#
# With --report it only analyzes existing Ginkgo JSON reports, for example
# test-results/k8s/report.json from a CI artifact.

set -euo pipefail

usage() {
	cat <<'EOF'
Usage: scripts/test-profile.sh [options]

Options:
  -s, --suite SUITE        k8s, cf or all (default: all)
  -n, --top N              number of slowest nodes to list (default: 5)
  -o, --out DIR            output directory (default: test-results/profile)
  -f, --focus REGEX        only run specs matching REGEX (-ginkgo.focus)
  -r, --report FILE        analyze an existing Ginkgo JSON report instead of
                           running a suite (repeatable)
      --cpuprofile         also record a Go CPU profile of the test process
      --interval SECONDS   cluster sampling interval (default: 2)
      --timeout DURATION   test binary timeout (default: 20m)
  -h, --help               show this help

Environment variables of the suites (MUTO_USE_EXISTING_CLUSTER, KUBECONFIG,
KIND_DEPLOYMENT_PATH, CF_E2E_*) are passed through unchanged.
EOF
}

SUITES="all"
TOP=5
OUT="test-results/profile"
FOCUS=""
REPORTS=()
CPUPROFILE=false
INTERVAL=2
TIMEOUT="20m"

while [ $# -gt 0 ]; do
	case "$1" in
	-s | --suite) SUITES="$2"; shift 2 ;;
	-n | --top) TOP="$2"; shift 2 ;;
	-o | --out) OUT="$2"; shift 2 ;;
	-f | --focus) FOCUS="$2"; shift 2 ;;
	-r | --report) REPORTS+=("$(cd "$(dirname "$2")" && pwd)/$(basename "$2")"); shift 2 ;;
	--cpuprofile) CPUPROFILE=true; shift ;;
	--interval) INTERVAL="$2"; shift 2 ;;
	--timeout) TIMEOUT="$2"; shift 2 ;;
	-h | --help) usage; exit 0 ;;
	*) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
	esac
done

case "$SUITES" in
all) SUITES="k8s cf" ;;
k8s | cf) ;;
*) echo "invalid suite: $SUITES (want k8s, cf or all)" >&2; exit 2 ;;
esac

command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }

cd "$(dirname "$0")/.."
REPO_ROOT=$(pwd)
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)

# Fractional seconds where date supports %N (GNU), whole seconds otherwise.
if [ "$(date +%N)" != "N" ]; then
	now() { date +%s.%N; }
else
	now() { date +%s; }
fi

GNU_TIME=""
for candidate in /usr/bin/time gtime; do
	if path=$(command -v "$candidate" 2>/dev/null) && "$path" --version 2>&1 | grep -q GNU; then
		GNU_TIME=$path
		break
	fi
done

DOCKER=false
if command -v docker >/dev/null && docker info >/dev/null 2>&1; then
	DOCKER=true
fi

SAMPLER_PID=""
stop_sampler() {
	if [ -n "$SAMPLER_PID" ]; then
		kill "$SAMPLER_PID" 2>/dev/null || true
		wait "$SAMPLER_PID" 2>/dev/null || true
		SAMPLER_PID=""
	fi
}
trap stop_sampler EXIT

# sample_cluster FILE appends "epoch<TAB>container<TAB>cpu%<TAB>mem MiB" rows for
# the k3s (testcontainers) and kind node containers until it is killed. The k3s
# container only appears once BeforeSuite starts it, so containers are
# rediscovered on every iteration.
sample_cluster() {
	local file=$1 ids
	while :; do
		ids=$(docker ps --format '{{.ID}} {{.Image}}' | awk '$2 ~ /rancher\/k3s|kindest\/node/ {print $1}')
		if [ -n "$ids" ]; then
			# shellcheck disable=SC2086 # word splitting of container IDs is intended
			docker stats --no-stream --format '{{.Name}}	{{.CPUPerc}}	{{.MemUsage}}' $ids 2>/dev/null |
				awk -F'\t' -v t="$(date +%s)" '
				function mib(v,   n, u) {
					n = v; sub(/[A-Za-z]+$/, "", n); u = v; sub(/^[0-9.]+/, "", u)
					if (u == "B") return n / 1048576
					if (u == "KiB" || u == "kB") return n / 1024
					if (u == "MiB" || u == "MB") return n + 0
					if (u == "GiB" || u == "GB") return n * 1024
					return 0
				}
				{ cpu = $2; sub(/%$/, "", cpu); split($3, m, " / "); printf "%s\t%s\t%.2f\t%.1f\n", t, $1, cpu, mib(m[1]) }' >>"$file"
		fi
		sleep "$INTERVAL"
	done
}

# report_tsv REPORT prints one row per executed node:
# suite, node type, state, seconds, start epoch, end epoch, serial, ordered, location, name
report_tsv() {
	jq -r '
	def epoch:
		capture("^(?<b>[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2})(?<f>[.][0-9]+)?(?<z>Z|[+-][0-9]{2}:[0-9]{2})$") as $c
		| ($c.b + "Z" | fromdateiso8601)
		+ (if $c.f then ("0" + $c.f | tonumber) else 0 end)
		- (if $c.z == "Z" then 0
		   else (if $c.z[0:1] == "-" then -1 else 1 end) * (($c.z[1:3] | tonumber) * 3600 + ($c.z[4:6] | tonumber) * 60)
		   end);
	.[] | .SuiteDescription as $suite
	| .SpecReports[]
	| select(.State != "skipped" and .State != "pending")
	| [
		$suite,
		.LeafNodeType,
		.State,
		(.RunTime / 1e9),
		(.StartTime | epoch),
		(.EndTime | epoch),
		(if .IsSerial then 1 else 0 end),
		(if .IsInOrderedContainer then 1 else 0 end),
		((.LeafNodeLocation.FileName | sub(".*/test/integration/"; "")) + ":" + (.LeafNodeLocation.LineNumber | tostring)),
		(if .LeafNodeType == "It" then ((.ContainerHierarchyTexts // []) + [.LeafNodeText] | join(" ")) else "[" + .LeafNodeType + "]" end)
	  ] | @tsv' "$1"
}

# analyze NAME DIR REPORT writes DIR/specs.tsv and DIR/profile.md. DIR may also
# contain build.txt, rusage.txt and cluster.tsv from a profiling run.
analyze() {
	local name=$1 dir=$2 report=$3
	report_tsv "$report" >"$dir/specs.tsv"
	local suite_seconds build_seconds="" wall="" user="" sys="" rss=""
	suite_seconds=$(jq '[.[].RunTime] | add / 1e9' "$report")
	[ -f "$dir/build.txt" ] && build_seconds=$(cat "$dir/build.txt")
	if [ -f "$dir/rusage.txt" ]; then
		wall=$(sed -n 's/^wall_s=//p' "$dir/rusage.txt")
		user=$(sed -n 's/^user_s=//p' "$dir/rusage.txt")
		sys=$(sed -n 's/^sys_s=//p' "$dir/rusage.txt")
		rss=$(sed -n 's/^max_rss_kb=//p' "$dir/rusage.txt")
	fi
	[ -f "$dir/cluster.tsv" ] || : >"$dir/cluster.tsv"

	awk -F'\t' -v name="$name" -v top="$TOP" -v suite_s="$suite_seconds" -v build_s="$build_seconds" \
		-v wall="$wall" -v user="$user" -v sys="$sys" -v rss="$rss" '
	function secs_or_na(v) { return v == "" ? "n/a" : sprintf("%.1f s", v) }
	function pct(a, b) { return b > 0 ? 100 * a / b : 0 }
	# First file: cluster samples (may be empty); sum containers per timestamp.
	FILENAME == ARGV[1] { cpu[$1] += $3; mem[$1] += $4; next }
	{
		n++; type[n] = $2; state[n] = $3; secs[n] = $4; start[n] = $5; stop[n] = $6
		serial += $7; ordered += $8; loc[n] = $9; spec[n] = $10
		if ($2 == "It") { specs_s += $4; its++ } else { setup_s += $4 }
		file = $9; sub(/:[0-9]+$/, "", file)
		if (!(file in by_file)) files[++nfiles] = file
		by_file[file] += $4; by_file_n[file]++
	}
	END {
		for (t in cpu) { samples++; cpu_sum += cpu[t]; if (cpu[t] > cpu_max) cpu_max = cpu[t]; if (mem[t] > mem_max) mem_max = mem[t] }

		print "## " name "\n"
		print "| Metric | Value |"
		print "|--------|-------|"
		printf "| Build test binary | %s |\n", secs_or_na(build_s)
		printf "| Suite run time | %.1f s |\n", suite_s
		printf "| Suite setup/teardown (Before/AfterSuite) | %.1f s (%.0f%%) |\n", setup_s, pct(setup_s, suite_s)
		printf "| Specs (%d executed) | %.1f s |\n", its, specs_s
		printf "| Specs marked Serial / in Ordered containers | %d / %d |\n", serial, ordered
		if (user != "") {
			printf "| Test process CPU (user + sys) | %.1f s + %.1f s |\n", user, sys
			printf "| Test process CPU utilization | %.0f%% of one core |\n", pct(user + sys, wall)
			printf "| Test process peak RSS | %.0f MiB |\n", rss / 1024
		}
		if (samples > 0) {
			printf "| Cluster CPU avg / peak (%d samples) | %.0f%% / %.0f%% of one core |\n", samples, cpu_sum / samples, cpu_max
			printf "| Cluster memory peak | %.0f MiB |\n", mem_max
		}

		# Selection sort for the top N; suites have well under 100 nodes.
		print "\n### Slowest nodes\n"
		print "| # | Node | Duration | Share | Cluster CPU avg | Location |"
		print "|---|------|----------|-------|-----------------|----------|"
		for (i = 1; i <= n; i++) used[i] = 0
		for (r = 1; r <= top && r <= n; r++) {
			best = 0
			for (i = 1; i <= n; i++) if (!used[i] && (best == 0 || secs[i] > secs[best])) best = i
			used[best] = 1
			c = 0; k = 0
			for (t in cpu) if (t + 0 >= start[best] && t + 0 <= stop[best]) { c += cpu[t]; k++ }
			printf "| %d | %s%s | %.2f s | %.0f%% | %s | `%s` |\n", r, spec[best], (state[best] == "passed" ? "" : " (" state[best] ")"), \
				secs[best], pct(secs[best], suite_s), (k > 0 ? sprintf("%.0f%%", c / k) : "n/a"), loc[best]
		}

		print "\n### Time by file\n"
		print "| File | Nodes | Total | Share |"
		print "|------|-------|-------|-------|"
		for (i = 1; i <= nfiles; i++) for (j = i + 1; j <= nfiles; j++) if (by_file[files[j]] > by_file[files[i]]) { f = files[i]; files[i] = files[j]; files[j] = f }
		for (i = 1; i <= nfiles; i++)
			printf "| `%s` | %d | %.1f s | %.0f%% |\n", files[i], by_file_n[files[i]], by_file[files[i]], pct(by_file[files[i]], suite_s)
		print ""
	}' "$dir/cluster.tsv" "$dir/specs.tsv" >"$dir/profile.md"

	if [ -f "$dir/cpu.pprof" ] && [ -f "$dir/$name.test" ]; then
		{
			printf '### Test process CPU profile (top 15)\n\n```\n'
			go tool pprof -top -nodecount=15 "$dir/$name.test" "$dir/cpu.pprof" 2>/dev/null | sed -n '1,25p'
			printf '```\n\n'
		} >>"$dir/profile.md"
	fi
}

# profile SUITE builds and runs one suite, then analyzes it. Returns the test
# binary's exit status.
profile() {
	local suite=$1 dir="$OUT/$1" status start
	local pkg="$REPO_ROOT/test/integration/$suite"
	rm -rf "$dir"
	mkdir -p "$dir"

	echo "==> [$suite] building test binary"
	start=$(now)
	go test -c -tags integration -o "$dir/$suite.test" "$pkg"
	awk -v a="$start" -v b="$(now)" 'BEGIN { printf "%.1f\n", b - a }' >"$dir/build.txt"

	local args=(-test.v -test.timeout "$TIMEOUT" -ginkgo.v -ginkgo.json-report="$dir/report.json")
	[ -n "$FOCUS" ] && args+=(-ginkgo.focus="$FOCUS")
	$CPUPROFILE && args+=(-test.cpuprofile="$dir/cpu.pprof")

	local timer=()
	if [ -n "$GNU_TIME" ]; then
		timer=("$GNU_TIME" -o "$dir/rusage.txt" -f 'wall_s=%e\nuser_s=%U\nsys_s=%S\nmax_rss_kb=%M')
	fi

	if [ "$suite" = "k8s" ] && $DOCKER; then
		sample_cluster "$dir/cluster.tsv" &
		SAMPLER_PID=$!
	fi

	echo "==> [$suite] running suite (log: $dir/test.log)"
	set +e
	# The suites resolve paths such as ../../../deploy/crds relative to the
	# package directory, so run the binary from there like go test does.
	(cd "$pkg" && ${timer[@]+"${timer[@]}"} "$dir/$suite.test" "${args[@]}") 2>&1 | tee "$dir/test.log"
	status=${PIPESTATUS[0]}
	set -e
	stop_sampler

	if [ -f "$dir/report.json" ]; then
		analyze "$suite" "$dir" "$dir/report.json"
	else
		echo "==> [$suite] no JSON report written; skipping analysis" >&2
	fi
	return "$status"
}

overall=0
DIRS=()
if [ ${#REPORTS[@]} -gt 0 ]; then
	i=0
	for report in "${REPORTS[@]}"; do
		i=$((i + 1))
		name=$(jq -r '.[0].SuiteDescription' "$report")
		dir="$OUT/report-$i"
		rm -rf "$dir"
		mkdir -p "$dir"
		analyze "$name" "$dir" "$report"
		DIRS+=("$dir")
	done
else
	[ -z "$GNU_TIME" ] && echo "note: GNU time not found (install gtime on macOS); skipping CPU and RSS metrics" >&2
	$DOCKER || echo "note: docker not available; skipping cluster CPU and memory sampling" >&2
	for suite in $SUITES; do
		profile "$suite" || overall=$?
		DIRS+=("$OUT/$suite")
	done
fi

{
	echo "# Test profile"
	echo
	echo "- Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
	echo "- Commit: $(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)"
	echo "- Host: $(uname -sm), $(getconf _NPROCESSORS_ONLN 2>/dev/null || echo '?') CPUs, $(go env GOVERSION 2>/dev/null || echo 'go ?')"
	echo "- CPU percentages are relative to one core; the test process includes the in-process controller manager."
	echo
	for dir in "${DIRS[@]}"; do
		[ -f "$dir/profile.md" ] && cat "$dir/profile.md"
	done
	if [ ${#DIRS[@]} -gt 1 ]; then
		echo "## Slowest nodes across all suites"
		echo
		echo "| # | Suite | Node | Duration | Location |"
		echo "|---|-------|------|----------|----------|"
		for dir in "${DIRS[@]}"; do
			[ -f "$dir/specs.tsv" ] && cat "$dir/specs.tsv"
		done | sort -t "$(printf '\t')" -k4,4gr |
			awk -F'\t' -v top="$TOP" 'NR <= top { printf "| %d | %s | %s | %.2f s | `%s` |\n", NR, $1, $10, $4, $9 }'
	fi
} >"$OUT/profile.md"

echo "==> report: $OUT/profile.md"
exit "$overall"
