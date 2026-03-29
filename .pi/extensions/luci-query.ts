import type { ExtensionAPI, ExtensionCommandContext } from "@mariozechner/pi-coding-agent";
import { BorderedLoader } from "@mariozechner/pi-coding-agent";
import { Key, matchesKey, SelectList, type SelectItem, truncateToWidth, fuzzyFilter } from "@mariozechner/pi-tui";

const LUCI_BASE_URL = "https://analysis.api.luci.app/prpc/luci.analysis.v1.TestHistory/";
const LUCI_PROJECT = "golang";

const PAGE_SIZE = 100;
const MAX_TEST_IDS = 500;
const MAX_VARIANTS = 3000;
const MAX_VERDICTS = 3000;
const MAX_TEST_CHOICES = 80;

type LuciVariantDef = Record<string, string>;

interface LuciQueryTestsResponse {
	testIds?: string[];
	nextPageToken?: string;
}

interface LuciVariantEntry {
	variantHash: string;
	variant?: {
		def?: LuciVariantDef;
	};
}

interface LuciQueryVariantsResponse {
	variants?: LuciVariantEntry[];
	nextPageToken?: string;
}

interface LuciVerdictEntry {
	testId?: string;
	variantHash?: string;
	invocationId?: string;
	status?: string;
	statusV2?: string;
	partitionTime?: string;
}

interface LuciQueryVerdictsResponse {
	verdicts?: LuciVerdictEntry[];
	nextPageToken?: string;
}

interface FetchPageResult<T> {
	items: T[];
	truncated: boolean;
}

interface VerdictRow {
	testId: string;
	status: string;
	variantHash: string;
	invocationId: string;
	partitionTime: string;
	builder: string;
	goos: string;
	goarch: string;
	branch: string;
	variantDef: LuciVariantDef;
	searchable: string;
}

type FieldName =
	| "status"
	| "builder"
	| "goos"
	| "goarch"
	| "branch"
	| "invocation"
	| "variant"
	| "test"
	| "time";

interface QueryToken {
	negated: boolean;
	field?: FieldName;
	matcher: TokenMatcher;
}

interface TokenMatcher {
	regex?: RegExp;
	needle?: string;
}

interface CommandLoadResult<T> {
	cancelled: boolean;
	value?: T;
	error?: string;
}

const FIELD_ALIASES: Record<string, FieldName> = {
	status: "status",
	s: "status",
	builder: "builder",
	b: "builder",
	goos: "goos",
	os: "goos",
	goarch: "goarch",
	arch: "goarch",
	branch: "branch",
	br: "branch",
	go_branch: "branch",
	invocation: "invocation",
	variant: "variant",
	v: "variant",
	test: "test",
	t: "test",
	time: "time",
};

function toErrorMessage(err: unknown): string {
	if (err instanceof Error) return err.message;
	return String(err);
}

function stripPrpcPrefix(body: string): string {
	return body.replace(/^\)\]\}'\n?/, "").trim();
}


function printableChunk(data: string): string {
	let chunk = "";
	for (const ch of data) {
		const code = ch.charCodeAt(0);
		if (code >= 32 && code !== 127) chunk += ch;
	}
	return chunk;
}

async function luciRpc<T>(method: string, payload: Record<string, unknown>, signal?: AbortSignal): Promise<T> {
	const response = await fetch(`${LUCI_BASE_URL}${method}`, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
			Accept: "application/json",
		},
		body: JSON.stringify(payload),
		signal,
	});

	if (!response.ok) {
		throw new Error(`LUCI ${method} failed: ${response.status} ${response.statusText}`);
	}

	const body = stripPrpcPrefix(await response.text());
	if (!body) {
		throw new Error(`LUCI ${method} returned an empty body`);
	}

	return JSON.parse(body) as T;
}

async function fetchPages<T>(
	method: string,
	payload: Record<string, unknown>,
	extract: (body: Record<string, unknown>) => T[],
	max: number,
	signal?: AbortSignal,
): Promise<FetchPageResult<T>> {
	let pageToken: string | undefined;
	const items: T[] = [];
	let truncated = false;

	do {
		const response = await luciRpc<Record<string, unknown>>(
			method,
			{ ...payload, pageSize: PAGE_SIZE, ...(pageToken ? { pageToken } : {}) },
			signal,
		);
		items.push(...extract(response));
		pageToken = response.nextPageToken as string | undefined;
		if (items.length >= max) {
			truncated = true;
			break;
		}
	} while (pageToken && !signal?.aborted);

	return { items: items.slice(0, max), truncated };
}

function queryTests(testIdSubstring: string, signal?: AbortSignal): Promise<FetchPageResult<string>> {
	return fetchPages("QueryTests", { project: LUCI_PROJECT, testIdSubstring }, (b) => (b.testIds as string[]) ?? [], MAX_TEST_IDS, signal);
}

function queryVariants(testId: string, signal?: AbortSignal): Promise<FetchPageResult<LuciVariantEntry>> {
	return fetchPages("QueryVariants", { project: LUCI_PROJECT, testId }, (b) => (b.variants as LuciVariantEntry[]) ?? [], MAX_VARIANTS, signal);
}

function queryVerdicts(testId: string, signal?: AbortSignal): Promise<FetchPageResult<LuciVerdictEntry>> {
	return fetchPages("Query", { project: LUCI_PROJECT, testId, predicate: {} }, (b) => (b.verdicts as LuciVerdictEntry[]) ?? [], MAX_VERDICTS, signal);
}

function looksLikeTestID(value: string): boolean {
	return value.includes(".") && value.includes("/");
}

function buildRows(testId: string, verdicts: LuciVerdictEntry[], variants: LuciVariantEntry[]): VerdictRow[] {
	const variantsByHash = new Map<string, LuciVariantDef>();

	for (const variant of variants) {
		if (!variant.variantHash) continue;
		variantsByHash.set(variant.variantHash, variant.variant?.def ?? {});
	}

	const rows: VerdictRow[] = [];

	for (const verdict of verdicts) {
		const variantHash = verdict.variantHash ?? "";
		const variantDef = variantsByHash.get(variantHash) ?? {};
		const status = verdict.statusV2 ?? verdict.status ?? "UNKNOWN";
		const invocationId = verdict.invocationId ?? "";
		const partitionTime = verdict.partitionTime ?? "";
		const builder = variantDef.builder ?? "";
		const goos = variantDef.goos ?? "";
		const goarch = variantDef.goarch ?? "";
		const branch = variantDef.go_branch ?? "";

		const searchable = [
			testId,
			status,
			invocationId,
			partitionTime,
			variantHash,
			builder,
			goos,
			goarch,
			branch,
			...Object.entries(variantDef).map(([key, value]) => `${key}:${value}`),
		]
			.filter(Boolean)
			.join(" ");

		rows.push({
			testId,
			status,
			variantHash,
			invocationId,
			partitionTime,
			builder,
			goos,
			goarch,
			branch,
			variantDef,
			searchable,
		});
	}

	rows.sort((a, b) => b.partitionTime.localeCompare(a.partitionTime));
	return rows;
}

function tokenizeQuery(query: string): string[] {
	const tokens: string[] = [];
	const regex = /"([^"]*)"|(\S+)/g;
	let match: RegExpExecArray | null;
	while ((match = regex.exec(query)) !== null) {
		tokens.push(match[1] ?? match[2] ?? "");
	}
	return tokens.filter(Boolean);
}

function parseTokenMatcher(value: string): TokenMatcher {
	const regexMatch = value.match(/^\/(.*)\/([a-z]*)$/i);
	if (regexMatch) {
		const pattern = regexMatch[1] ?? "";
		const rawFlags = regexMatch[2] ?? "";
		const flags = rawFlags.includes("i") ? rawFlags.replace(/g/g, "") : `${rawFlags.replace(/g/g, "")}i`;
		return { regex: new RegExp(pattern, flags) };
	}
	return { needle: value.toLowerCase() };
}

function parseQuery(query: string): QueryToken[] {
	const tokens = tokenizeQuery(query.trim());
	const parsed: QueryToken[] = [];

	for (const rawToken of tokens) {
		let token = rawToken;
		let negated = false;
		if (token.startsWith("-")) {
			negated = true;
			token = token.slice(1);
		}
		if (!token) continue;

		let field: FieldName | undefined;
		let value = token;
		const colon = token.indexOf(":");

		if (colon > 0 && colon < token.length - 1) {
			const maybeField = FIELD_ALIASES[token.slice(0, colon).toLowerCase()];
			if (maybeField) {
				field = maybeField;
				value = token.slice(colon + 1);
			}
		}

		parsed.push({
			negated,
			field,
			matcher: parseTokenMatcher(value),
		});
	}

	return parsed;
}

function getField(row: VerdictRow, field: FieldName): string {
	switch (field) {
		case "status":
			return row.status;
		case "builder":
			return row.builder;
		case "goos":
			return row.goos;
		case "goarch":
			return row.goarch;
		case "branch":
			return row.branch;
		case "invocation":
			return row.invocationId;
		case "variant":
			return row.variantHash;
		case "test":
			return row.testId;
		case "time":
			return row.partitionTime;
		default: {
			const _never: never = field;
			throw new Error(`Unhandled field: ${String(_never)}`);
		}
	}
}

function tokenMatches(token: QueryToken, haystack: string): boolean {
	if (token.matcher.regex) {
		return token.matcher.regex.test(haystack);
	}
	if (token.matcher.needle) {
		return haystack.toLowerCase().includes(token.matcher.needle);
	}
	return true;
}

function matchesRow(row: VerdictRow, tokens: QueryToken[]): boolean {
	for (const token of tokens) {
		const haystack = token.field ? getField(row, token.field) : row.searchable;
		const matched = tokenMatches(token, haystack);
		if (token.negated ? matched : !matched) return false;
	}
	return true;
}

function shortTime(timestamp: string): string {
	if (!timestamp) return "?";
	return timestamp.replace("T", " ").replace("Z", "").slice(0, 19);
}

function summarizeStatuses(rows: VerdictRow[]): string {
	const counts = new Map<string, number>();
	for (const row of rows) {
		counts.set(row.status, (counts.get(row.status) ?? 0) + 1);
	}

	const preferred = ["FAILED", "FLAKY", "PASSED", "SKIPPED", "UNKNOWN"];
	const remaining = Array.from(counts.keys()).filter((status) => !preferred.includes(status)).sort();
	const ordered = [...preferred.filter((status) => counts.has(status)), ...remaining];

	return ordered.map((status) => `${status}:${counts.get(status)}`).join(" ");
}

async function runWithLoader<T>(
	ctx: ExtensionCommandContext,
	message: string,
	task: (signal: AbortSignal) => Promise<T>,
): Promise<CommandLoadResult<T>> {
	const result = await ctx.ui.custom<CommandLoadResult<T>>((tui, theme, _kb, done) => {
		const loader = new BorderedLoader(tui, theme, message);
		loader.onAbort = () => done({ cancelled: true });

		task(loader.signal)
			.then((value) => done({ cancelled: false, value }))
			.catch((err) => done({ cancelled: false, error: toErrorMessage(err) }));

		return loader;
	});

	return result;
}

async function pickTestIDMatch(
	ctx: ExtensionCommandContext,
	seed: string,
	matches: string[],
	totalMatches: number,
	truncated: boolean,
): Promise<string | null> {
	return ctx.ui.custom<string | null>((tui, theme, _kb, done) => {
		let filter = "";
		let cursorPos = 0;
		const items: SelectItem[] = matches.map((match) => ({ value: match, label: match }));
		const visibleCount = Math.max(3, Math.min(items.length, tui.terminal.rows - 8));
		let filteredItems = items;
		let selectedIndex = 0;
		let selectList: SelectList;

		function pageStep(): number {
			return Math.max(1, Math.floor(visibleCount / 2));
		}

		function rebuildSelectList() {
			selectList = new SelectList(filteredItems, visibleCount, {
				selectedPrefix: (text: string) => theme.fg("accent", text),
				selectedText: (text: string) => theme.fg("accent", text),
				description: (text: string) => theme.fg("muted", text),
				scrollInfo: (text: string) => theme.fg("dim", text),
				noMatch: (text: string) => theme.fg("warning", text),
			});
			selectList.onSelect = (item: SelectItem) => done(item.value);
			selectList.onCancel = () => done(null);
			selectList.onSelectionChange = (item: SelectItem) => {
				selectedIndex = filteredItems.findIndex((candidate) => candidate.value === item.value);
			};
			selectList.setSelectedIndex(selectedIndex);
		}

		function setSelection(index: number) {
			if (filteredItems.length === 0) {
				selectedIndex = 0;
				rebuildSelectList();
				return;
			}
			selectedIndex = Math.max(0, Math.min(index, filteredItems.length - 1));
			selectList.setSelectedIndex(selectedIndex);
		}

		function applyFilter() {
			filteredItems = filter === "" ? items : fuzzyFilter(items, filter, (item: SelectItem) => item.value);
			selectedIndex = 0;
			rebuildSelectList();
		}

		rebuildSelectList();

		return {
			handleInput(data: string) {
				if (matchesKey(data, Key.ctrl("u"))) {
					setSelection(selectedIndex - pageStep());
					tui.requestRender();
					return;
				}

				if (matchesKey(data, Key.ctrl("d"))) {
					setSelection(selectedIndex + pageStep());
					tui.requestRender();
					return;
				}

				if (
					matchesKey(data, Key.up) ||
					matchesKey(data, Key.down) ||
					matchesKey(data, Key.enter) ||
					matchesKey(data, Key.escape)
				) {
					selectList.handleInput(data);
					tui.requestRender();
					return;
				}

				if (matchesKey(data, Key.left)) {
					cursorPos = Math.max(0, cursorPos - 1);
					tui.requestRender();
					return;
				}

				if (matchesKey(data, Key.right)) {
					cursorPos = Math.min(filter.length, cursorPos + 1);
					tui.requestRender();
					return;
				}

				if (matchesKey(data, Key.backspace)) {
					if (cursorPos > 0) {
						filter = filter.slice(0, cursorPos - 1) + filter.slice(cursorPos);
						cursorPos--;
						applyFilter();
						tui.requestRender();
					}
					return;
				}

				if (matchesKey(data, Key.ctrl("k"))) {
					if (filter.length > 0) {
						filter = "";
						cursorPos = 0;
						applyFilter();
						tui.requestRender();
					}
					return;
				}

				const chunk = printableChunk(data);
				if (chunk.length > 0) {
					filter = filter.slice(0, cursorPos) + chunk + filter.slice(cursorPos);
					cursorPos += chunk.length;
					applyFilter();
					tui.requestRender();
				}
			},
			render(width: number): string[] {
				const safeWidth = Math.max(1, width);
				const lines: string[] = [];
				const push = (line: string) => lines.push(truncateToWidth(line, safeWidth, "…"));

				push("-".repeat(safeWidth));
				push("Select LUCI test ID");
				push(`query: ${seed}`);
				push(
					`matches: ${totalMatches}${truncated ? ", truncated" : ""}${totalMatches > matches.length ? `, showing first ${matches.length}` : ""}`,
				);
				const renderedFilter = `${filter.slice(0, cursorPos)}|${filter.slice(cursorPos)}`;
				push(`filter: ${filter === "" ? "| (fuzzy match)" : renderedFilter}`);
				lines.push(...selectList.render(safeWidth));
				push("");
				push("keys: type/backspace fuzzy-filter • Ctrl+K clear • ↑↓ move • Ctrl+U/D half-page • Enter select • Esc cancel");
				push("-".repeat(safeWidth));

				return lines;
			},
			invalidate() {},
		};
	});
}

async function pickTestID(ctx: ExtensionCommandContext, seed: string): Promise<string | null> {
	const trimmed = seed.trim();
	if (!trimmed) return null;

	const lookup = await runWithLoader(ctx, `Searching LUCI tests for \"${trimmed}\"...`, (signal) =>
		queryTests(trimmed, signal),
	);

	if (lookup.cancelled) return null;
	if (lookup.error) throw new Error(lookup.error);

	const response = lookup.value;
	if (!response) return null;

	const matches = response.items;
	const exactMatch = matches.find((item) => item === trimmed);

	if (exactMatch) return exactMatch;
	if (matches.length === 1) return matches[0];

	if (matches.length === 0) {
		if (looksLikeTestID(trimmed)) return trimmed;
		ctx.ui.notify(`No LUCI tests found for \"${trimmed}\"`, "warning");
		return null;
	}

	const candidates = matches.slice(0, MAX_TEST_CHOICES);
	return pickTestIDMatch(ctx, trimmed, candidates, matches.length, response.truncated);
}

async function showFilterUI(
	ctx: ExtensionCommandContext,
	testId: string,
	rows: VerdictRow[],
	notes: string[],
): Promise<VerdictRow | null> {
	if (!ctx.hasUI) return null;

	return ctx.ui.custom<VerdictRow | null>((tui, _theme, _kb, done) => {
		let query = "";
		let queryCursor = 0;
		let filterError: string | undefined;
		let filteredRows = [...rows];
		let selectedIndex = 0;
		let scrollOffset = 0;

		function maxVisibleRows(): number {
			const chromeLines = 17 + notes.length + (filterError ? 1 : 0);
			return Math.max(3, Math.min(24, tui.terminal.rows - chromeLines));
		}

		function clampSelection() {
			if (filteredRows.length === 0) {
				selectedIndex = 0;
				scrollOffset = 0;
				return;
			}

			selectedIndex = Math.max(0, Math.min(selectedIndex, filteredRows.length - 1));
			const maxVisible = maxVisibleRows();
			if (selectedIndex < scrollOffset) scrollOffset = selectedIndex;
			if (selectedIndex >= scrollOffset + maxVisible) {
				scrollOffset = selectedIndex - maxVisible + 1;
			}
			scrollOffset = Math.max(0, scrollOffset);
		}

		function pageStep(): number {
			return Math.max(1, Math.floor(maxVisibleRows() / 2));
		}

		function applyFilter(resetSelection: boolean) {
			const rawQuery = query.trim();
			if (!rawQuery) {
				filterError = undefined;
				filteredRows = [...rows];
				if (resetSelection) selectedIndex = 0;
				clampSelection();
				return;
			}

			try {
				const tokens = parseQuery(rawQuery);
				filteredRows = rows.filter((row) => matchesRow(row, tokens));
				filterError = undefined;
			} catch (err) {
				filteredRows = [];
				filterError = toErrorMessage(err);
			}

			if (resetSelection) selectedIndex = 0;
			clampSelection();
		}

		applyFilter(true);

		return {
			handleInput(data: string) {
				if (matchesKey(data, Key.escape)) {
					done(null);
					return;
				}

				if (matchesKey(data, Key.enter)) {
					const selected = filteredRows[selectedIndex];
					if (selected) done(selected);
					return;
				}

				if (matchesKey(data, Key.ctrl("u"))) {
					if (filteredRows.length > 0) {
						selectedIndex -= pageStep();
						clampSelection();
						tui.requestRender();
					}
					return;
				}

				if (matchesKey(data, Key.ctrl("d"))) {
					if (filteredRows.length > 0) {
						selectedIndex += pageStep();
						clampSelection();
						tui.requestRender();
					}
					return;
				}

				if (matchesKey(data, Key.up)) {
					if (filteredRows.length > 0) {
						selectedIndex = Math.max(0, selectedIndex - 1);
						clampSelection();
						tui.requestRender();
					}
					return;
				}

				if (matchesKey(data, Key.down)) {
					if (filteredRows.length > 0) {
						selectedIndex = Math.min(filteredRows.length - 1, selectedIndex + 1);
						clampSelection();
						tui.requestRender();
					}
					return;
				}

				if (matchesKey(data, Key.home)) {
					selectedIndex = 0;
					clampSelection();
					tui.requestRender();
					return;
				}

				if (matchesKey(data, Key.end)) {
					selectedIndex = Math.max(0, filteredRows.length - 1);
					clampSelection();
					tui.requestRender();
					return;
				}

				if (matchesKey(data, Key.left)) {
					queryCursor = Math.max(0, queryCursor - 1);
					tui.requestRender();
					return;
				}

				if (matchesKey(data, Key.right)) {
					queryCursor = Math.min(query.length, queryCursor + 1);
					tui.requestRender();
					return;
				}

				if (matchesKey(data, Key.backspace)) {
					if (queryCursor > 0) {
						query = query.slice(0, queryCursor - 1) + query.slice(queryCursor);
						queryCursor--;
						applyFilter(true);
						tui.requestRender();
					}
					return;
				}

				if (matchesKey(data, Key.ctrl("k"))) {
					if (query.length > 0) {
						query = "";
						queryCursor = 0;
						applyFilter(true);
						tui.requestRender();
					}
					return;
				}

				const chunk = printableChunk(data);
				if (chunk.length > 0) {
					query = query.slice(0, queryCursor) + chunk + query.slice(queryCursor);
					queryCursor += chunk.length;
					applyFilter(true);
					tui.requestRender();
				}
			},
			render(width: number): string[] {
				const safeWidth = Math.max(1, width);
				const lines: string[] = [];
				const push = (line: string) => lines.push(truncateToWidth(line, safeWidth, "…"));

				push("-".repeat(safeWidth));
				push("LUCI test summary");
				push(`test: ${testId}`);
				push(`${rows.length} verdicts loaded • ${filteredRows.length} matching`);
				push(`status: ${summarizeStatuses(filteredRows)}`);
				for (const note of notes) {
					push(`note: ${note}`);
				}
				push("");
				push("query (field:value, -negation, /regex/):");
				push(`> ${query.slice(0, queryCursor)}|${query.slice(queryCursor)}`);
				if (filterError) {
					push(`query error: ${filterError}`);
				}
				push("");

				if (filteredRows.length === 0) {
					push("No results");
				} else {
					const maxVisible = maxVisibleRows();
					const start = Math.min(scrollOffset, Math.max(0, filteredRows.length - maxVisible));
					const end = Math.min(filteredRows.length, start + maxVisible);
					const statusWidth = 8;
					const timeWidth = 19;
					const platformWidth = 13;
					const cell = (text: string, width: number) => truncateToWidth(text, width, "", true);
					push(`  ${cell("STATUS", statusWidth)} ${cell("TIME", timeWidth)} ${cell("PLATFORM", platformWidth)} BUILDER`);

					for (let i = start; i < end; i++) {
						const row = filteredRows[i];
						const selected = i === selectedIndex ? ">" : " ";
						const status = cell(row.status, statusWidth);
						const platform = cell(`${row.goos || "?"}/${row.goarch || "?"}`, platformWidth);
						const line = `${selected} ${status} ${shortTime(row.partitionTime)} ${platform} ${row.builder || row.variantHash}`;
						push(line);
					}

					if (filteredRows.length > maxVisible) {
						push(`showing ${start + 1}-${end} of ${filteredRows.length}`);
					}

					const selectedRow = filteredRows[selectedIndex];
					if (selectedRow) {
						push("");
						push(`builder: ${selectedRow.builder || "?"}`);
						push(`branch: ${selectedRow.branch || "?"}`);
						push(`invocation: ${selectedRow.invocationId || "?"}`);
						push(`variant: ${selectedRow.variantHash || "?"}`);
					}
				}

				push("");
				push("fields: status builder goos goarch branch invocation variant test time");
				push("keys: type/backspace filter • Ctrl+K clear • ↑↓ move • Ctrl+U/D half-page • Enter select • Esc change test");
				push("-".repeat(safeWidth));

				return lines;
			},
			invalidate() {},
		};
	});
}

async function retryOrQuit(ctx: ExtensionCommandContext, defaultValue: string): Promise<string | null> {
	const next = await ctx.ui.editor("Change LUCI test id or substring (Esc to quit)", defaultValue);
	if (next === undefined) return null;
	return next.trim() || null;
}

export default function luciSkimExtension(pi: ExtensionAPI) {
	pi.registerCommand("luci", {
		description: "Skim LUCI test verdicts with local live filtering",
		handler: async (args, ctx) => {
			if (!ctx.hasUI) {
				ctx.ui.notify("/luci requires interactive mode", "error");
				return;
			}

			const seedArg = args.trim();
			let seed = seedArg || (await ctx.ui.input("LUCI test id or substring", "e.g. cmd/go.TestScript/list_swigcxx"));
			if (!seed) return;
			seed = seed.trim();
			if (!seed) return;

			for (;;) {
				let testId: string | null;
				try {
					testId = await pickTestID(ctx, seed);
				} catch (err) {
					ctx.ui.notify(toErrorMessage(err), "error");
					seed = (await retryOrQuit(ctx, seed)) ?? "";
					if (!seed) return;
					continue;
				}
				if (!testId) return;

				const history = await runWithLoader(ctx, `Fetching LUCI history for ${testId}...`, async (signal) => {
					const [variants, verdicts] = await Promise.all([queryVariants(testId, signal), queryVerdicts(testId, signal)]);
					return { variants, verdicts };
				});

				if (history.cancelled) return;
				if (history.error) {
					ctx.ui.notify(history.error, "error");
					seed = (await retryOrQuit(ctx, testId)) ?? "";
					if (!seed) return;
					continue;
				}

				if (!history.value) {
					ctx.ui.notify("Failed to load LUCI history", "error");
					seed = (await retryOrQuit(ctx, testId)) ?? "";
					if (!seed) return;
					continue;
				}

				const rows = buildRows(testId, history.value.verdicts.items, history.value.variants.items);
				if (rows.length === 0) {
					ctx.ui.notify(`No verdicts found for ${testId}`, "warning");
					seed = (await retryOrQuit(ctx, testId)) ?? "";
					if (!seed) return;
					continue;
				}

				const notes: string[] = [];
				if (history.value.verdicts.truncated) {
					notes.push(`verdicts capped at ${MAX_VERDICTS}`);
				}
				if (history.value.variants.truncated) {
					notes.push(`variants capped at ${MAX_VARIANTS}`);
				}

				const selected = await showFilterUI(ctx, testId, rows, notes);
				if (selected) {
					ctx.ui.notify(
						`${selected.status} • ${selected.builder || selected.variantHash} • ${selected.invocationId || "(no invocation id)"}`,
						"info",
					);
					return;
				}

				seed = (await retryOrQuit(ctx, testId)) ?? "";
				if (!seed) return;
			}
		},
	});
}
