// The page's half of the app. Everything under Converter and events is
// generated from internal/desktop by `mygo generate`; nothing here calls Go by
// name.
import { Channel, isCallError, onFileDrop } from "mygo-runtime";
import { Converter, events } from "./mygo";
import type { Report, Result } from "./mygo";

const $ = <T extends HTMLElement>(selector: string): T => {
  const element = document.querySelector<T>(selector);
  if (!element) {
    throw new Error(`missing element ${selector}`);
  }
  return element;
};

// How each outcome of the Go side reads on screen.
const OUTCOME_LABEL: Record<string, string> = {
  planned: "将转换",
  converted: "已转换",
  skipped: "已跳过",
  failed: "失败",
};

const addFiles = $("#add-files") as HTMLButtonElement;
const addDir = $("#add-dir") as HTMLButtonElement;
const clearButton = $("#clear") as HTMLButtonElement;
const queue = $("#queue") as HTMLUListElement;
const queueEmpty = $("#queue-empty") as HTMLParagraphElement;
const stats = $("#stats") as HTMLParagraphElement;

const output = $("#output") as HTMLInputElement;
const pickOutput = $("#pick-output") as HTMLButtonElement;
const template = $("#template") as HTMLInputElement;
const depth = $("#depth") as HTMLInputElement;
const threads = $("#threads") as HTMLInputElement;
const tag = $("#tag") as HTMLInputElement;
const skipExisting = $("#skip-existing") as HTMLInputElement;
const dryRun = $("#dry-run") as HTMLInputElement;

const runButton = $("#run") as HTMLButtonElement;
const cancelButton = $("#cancel") as HTMLButtonElement;
const openOutput = $("#open-output") as HTMLButtonElement;
const progress = $("#progress") as HTMLProgressElement;
const summary = $("#summary") as HTMLParagraphElement;
const log = $("#log") as HTMLUListElement;

// The queued inputs, in the order they were added, without duplicates.
const inputs: string[] = [];

let running = false;
// Where the run wrote, so "open the output directory" has somewhere to go.
let outputDir: string | null = null;
// The first destination a report named, used when no output directory was set.
let firstDest: string | null = null;
// Guards against a slow scan overwriting the result of a newer one.
let scanToken = 0;

const queueControls = [addFiles, addDir, clearButton];
const optionControls = [output, pickOutput, template, depth, threads, tag, skipExisting, dryRun];

const message = (error: unknown): string => (isCallError(error) ? error.message : String(error));

// guard runs an action that talks to Go, reporting a failure in the summary
// rather than leaving an unhandled rejection.
async function guard(action: () => Promise<void>): Promise<void> {
  try {
    await action();
  } catch (error) {
    summary.textContent = `操作失败：${message(error)}`;
  }
}

function numberValue(element: HTMLInputElement, fallback: number): number {
  const value = Number.parseInt(element.value, 10);
  return Number.isNaN(value) ? fallback : value;
}

function dirname(path: string): string {
  const cut = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  return cut > 0 ? path.slice(0, cut) : path;
}

function renderQueue(): void {
  queue.replaceChildren();
  queue.hidden = inputs.length === 0;
  queueEmpty.hidden = inputs.length > 0;

  inputs.forEach((input, index) => {
    const row = document.createElement("li");
    row.className = "queue-row";

    const path = document.createElement("span");
    path.className = "queue-path";
    path.textContent = input;
    path.title = input;

    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "ghost small";
    remove.textContent = "移除";
    remove.disabled = running;
    remove.addEventListener("click", () => {
      inputs.splice(index, 1);
      renderQueue();
      void refreshStats();
    });

    row.append(path, remove);
    queue.append(row);
  });
}

function addInputs(paths: string[]): void {
  let changed = false;
  for (const path of paths) {
    const value = path.trim();
    if (value === "" || inputs.includes(value)) {
      continue;
    }
    inputs.push(value);
    changed = true;
  }
  if (changed) {
    renderQueue();
    void refreshStats();
  }
}

// refreshStats shows how many containers the queue holds, using the same
// traversal a run would, so the number matches what a run will handle.
async function refreshStats(): Promise<void> {
  const token = ++scanToken;
  if (inputs.length === 0) {
    stats.textContent = "";
    return;
  }
  stats.textContent = "正在统计…";
  try {
    const found = await Converter.scan([...inputs], numberValue(depth, 0));
    if (token === scanToken) {
      stats.textContent = `${inputs.length} 个来源 · ${found.length} 个 .ncm`;
    }
  } catch {
    if (token === scanToken) {
      stats.textContent = "";
    }
  }
}

// setRunning locks the queue and the options while a run is in flight, so they
// cannot change under it.
function setRunning(value: boolean): void {
  running = value;
  for (const control of [...queueControls, ...optionControls, runButton]) {
    control.disabled = value;
  }
  cancelButton.disabled = !value;
  renderQueue();
}

function appendLog(report: Report): void {
  const row = document.createElement("li");
  row.className = `log-row ${report.outcome}`;

  const badge = document.createElement("span");
  badge.className = "badge";
  badge.textContent = OUTCOME_LABEL[report.outcome] ?? report.outcome;

  const text = document.createElement("span");
  text.className = "log-text";
  text.textContent = report.dest ? `${report.source} → ${report.dest}` : report.source;
  text.title = text.textContent;

  row.append(badge, text);

  if (report.error) {
    const error = document.createElement("span");
    error.className = "log-error";
    error.textContent = report.error;
    row.append(error);
  }

  log.append(row);
  log.scrollTop = log.scrollHeight;

  if (report.dest && firstDest === null) {
    firstDest = report.dest;
  }
}

function showSummary(result: Result): void {
  const parts = [
    `共 ${result.total} 个`,
    `转换 ${result.converted}`,
    `跳过 ${result.skipped}`,
    `失败 ${result.failed}`,
    `${(result.durationMs / 1000).toFixed(1)}s`,
  ];
  if (result.dryRun) {
    parts.push("试运行");
  }
  if (result.canceled) {
    parts.push("已取消");
  }
  summary.textContent = parts.join(" · ");
}

async function onRun(): Promise<void> {
  if (running || inputs.length === 0) {
    return;
  }

  log.replaceChildren();
  firstDest = null;
  outputDir = null;
  openOutput.disabled = true;

  const requested = output.value.trim();
  const depthValue = numberValue(depth, 0);

  // The progress bar counts against what the run will handle, which is what a
  // dry run lists as well.
  let planned = 0;
  try {
    planned = (await Converter.scan([...inputs], depthValue)).length;
  } catch {
    planned = 0;
  }
  progress.max = Math.max(planned, 1);
  progress.value = 0;
  summary.textContent = "转换中…";

  setRunning(true);

  // Each report arrives as its container is handled, before the call settles.
  const reports = new Channel<Report>((report) => {
    progress.value += 1;
    appendLog(report);
  });

  const request = {
    inputs: [...inputs],
    output: requested,
    outputTemplate: template.value.trim(),
    tag: tag.checked,
    depth: depthValue,
    threads: numberValue(threads, 1),
    dryRun: dryRun.checked,
    skipExisting: skipExisting.checked,
  };

  try {
    const result = await Converter.run(request, reports);
    showSummary(result);
    if (requested !== "") {
      outputDir = requested;
    } else if (firstDest !== null) {
      outputDir = dirname(firstDest);
    }
    openOutput.disabled = outputDir === null;
  } catch (error) {
    summary.textContent = `运行失败：${message(error)}`;
  } finally {
    setRunning(false);
  }
}

addFiles.addEventListener("click", () =>
  guard(async () => {
    addInputs(await Converter.pickFiles());
  }),
);

addDir.addEventListener("click", () =>
  guard(async () => {
    const directory = await Converter.pickDirectory();
    if (directory !== "") {
      addInputs([directory]);
    }
  }),
);

clearButton.addEventListener("click", () => {
  inputs.length = 0;
  renderQueue();
  void refreshStats();
});

pickOutput.addEventListener("click", () =>
  guard(async () => {
    const directory = await Converter.pickOutput();
    if (directory !== "") {
      output.value = directory;
    }
  }),
);

runButton.addEventListener("click", () => void onRun());

cancelButton.addEventListener("click", () => {
  void Converter.cancel();
});

openOutput.addEventListener("click", () =>
  guard(async () => {
    if (outputDir !== null) {
      await Converter.openPath(outputDir);
    }
  }),
);

depth.addEventListener("input", () => void refreshStats());

// Files dropped on the window, and the files chosen from the app menu, end up
// in the same queue as the buttons.
onFileDrop(({ paths }) => addInputs(paths));
events.filesOpened.on((paths) => addInputs(paths));

// The build the page is talking to. Outside MyGo (a plain browser tab) the call
// rejects, which leaves the header empty rather than breaking the page.
void (async () => {
  try {
    const info = await Converter.info();
    const version = info.version === "" ? "" : ` v${info.version}`;
    $("#build").textContent = `${info.name}${version} · mygo ${info.myGo} · ${info.platform}/${info.arch}`;
  } catch {
    // Not running in a MyGo window; nothing to show.
  }
})();
