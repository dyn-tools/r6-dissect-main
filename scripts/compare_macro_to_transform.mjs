import fs from "fs";
import path from "path";

function usage() {
  console.error("Usage: node scripts/compare_macro_to_transform.mjs <macro.txt> <transforms.json>");
  process.exit(1);
}

if (process.argv.length < 4) {
  usage();
}

const macroPath = process.argv[2];
const transformsPath = process.argv[3];

const macro = JSON.parse(fs.readFileSync(macroPath, "utf8"));
const transforms = JSON.parse(fs.readFileSync(transformsPath, "utf8"));
const track = (transforms.primaryTracks && transforms.primaryTracks[0]) || (transforms.tracks && transforms.tracks[0]);

if (!track || !Array.isArray(track.samples) || track.samples.length === 0) {
  console.error("No samples found in transform export.");
  process.exit(2);
}

const firstTimed = track.samples.find((sample) => typeof sample.timeInSeconds === "number");
if (!firstTimed) {
  console.error("Transform export has no replay clock attached.");
  process.exit(3);
}

const replayStartSeconds = firstTimed.timeInSeconds;

function parseMs(value) {
  if (typeof value === "number") {
    return value;
  }
  const match = /^(-?\d+(?:\.\d+)?)ms$/.exec(String(value).trim());
  if (!match) {
    throw new Error(`Unsupported time format: ${value}`);
  }
  return Number(match[1]);
}

function normalizeCommands(commands) {
  const normalized = commands.map((command, index) => {
    const startMs = parseMs(command.at ?? 0);
    const durationMs = parseMs(command.duration ?? 0);
    const endMs = startMs + durationMs;
    const dx = Number(command.dx ?? 0);
    const dy = Number(command.dy ?? 0);
    const keys = Array.isArray(command.keys)
      ? command.keys
      : command.key
        ? [command.key]
        : [];
    return {
      index,
      label: command.label || `${command.type || "command"}-${index + 1}`,
      type: command.type || "unknown",
      startMs,
      endMs,
      durationMs,
      dx,
      dy,
      keys,
    };
  });
  const movementIntervals = normalized
    .filter((command) => command.keys.some((key) => ["w", "a", "s", "d", "shift", "ctrl", "space"].includes(String(key).toLowerCase())))
    .map((command) => ({ startMs: command.startMs, endMs: command.endMs }));
  const mouseIntervals = [];
  const windows = normalized.map((command) => ({
    ...command,
    movementActive: movementIntervals.some((interval) => !(interval.endMs < command.startMs || interval.startMs > command.endMs)),
  })).flatMap((command) => {
    if (command.type !== "mousemove") {
      return [];
    }
    mouseIntervals.push({ startMs: command.startMs, endMs: command.endMs });
    return [command];
  });
  let moveIndex = 0;
  for (const movement of movementIntervals) {
    let segments = [movement];
    for (const mouse of mouseIntervals) {
      const next = [];
      for (const segment of segments) {
        if (mouse.endMs <= segment.startMs || mouse.startMs >= segment.endMs) {
          next.push(segment);
          continue;
        }
        if (mouse.startMs > segment.startMs) {
          next.push({ startMs: segment.startMs, endMs: Math.min(mouse.startMs, segment.endMs) });
        }
        if (mouse.endMs < segment.endMs) {
          next.push({ startMs: Math.max(mouse.endMs, segment.startMs), endMs: segment.endMs });
        }
      }
      segments = next;
    }
    for (const segment of segments) {
      if ((segment.endMs - segment.startMs) < 150) {
        continue;
      }
      moveIndex += 1;
      windows.push({
        index: normalized.length + moveIndex,
        label: `move-${moveIndex}`,
        type: "holdkey",
        startMs: segment.startMs,
        endMs: segment.endMs,
        durationMs: segment.endMs - segment.startMs,
        dx: 0,
        dy: 0,
        keys: ["w"],
        movementActive: true,
      });
    }
  }
  return windows.sort((left, right) => left.startMs - right.startMs || left.endMs - right.endMs);
}

function classifyWindow(command) {
  const hasHorizontalMouse = command.dx !== 0 && command.dy === 0;
  const hasVerticalMouse = command.dy !== 0 && command.dx === 0;
  const hasMovement = command.keys.some((key) => ["w", "a", "s", "d", "shift", "ctrl"].includes(String(key).toLowerCase()));
  if (command.type === "mousemove" && hasHorizontalMouse && command.movementActive) {
    return "move+yaw";
  }
  if (command.type === "mousemove" && hasVerticalMouse && command.movementActive) {
    return "move+pitch";
  }
  if (command.type === "mousemove" && command.dx !== 0 && command.dy !== 0 && command.movementActive) {
    return "move+combined";
  }
  if (command.type === "mousemove" && hasHorizontalMouse && !hasMovement) {
    return "yaw-only";
  }
  if (command.type === "mousemove" && hasVerticalMouse && !hasMovement) {
    return "pitch-only";
  }
  if (hasMovement && (command.dx !== 0 || command.dy !== 0)) {
    return "move+mouse";
  }
  if (hasMovement) {
    return "move-only";
  }
  return "other";
}

function unwrap(values) {
  if (values.length === 0) {
    return [];
  }
  const result = [values[0]];
  for (let index = 1; index < values.length; index++) {
    let current = values[index];
    let delta = current - result[index - 1];
    while (delta > 180) {
      current -= 360;
      delta = current - result[index - 1];
    }
    while (delta < -180) {
      current += 360;
      delta = current - result[index - 1];
    }
    result.push(current);
  }
  return result;
}

function span(values) {
  if (!values.length) {
    return null;
  }
  const min = Math.min(...values);
  const max = Math.max(...values);
  return {
    min,
    max,
    span: max - min,
  };
}

function sampleElapsedMs(sample) {
  return (replayStartSeconds - sample.timeInSeconds) * 1000;
}

const commands = normalizeCommands(macro.commands);
const windows = commands.map((command) => {
  const samples = track.samples.filter((sample) => {
    if (typeof sample.timeInSeconds !== "number") {
      return false;
    }
    const elapsedMs = sampleElapsedMs(sample);
    return elapsedMs >= command.startMs && elapsedMs <= command.endMs;
  });
  const axisValues = (getter) => samples.map(getter).filter((value) => typeof value === "number");
  const view = unwrap(axisValues((sample) => sample.viewingDirectionDegrees));
  const viewPitch = unwrap(axisValues((sample) => sample.viewPitchDegrees));
  const rotX = unwrap(axisValues((sample) => sample.rotationDegrees?.x));
  const rotY = unwrap(axisValues((sample) => sample.rotationDegrees?.y));
  const rotZ = unwrap(axisValues((sample) => sample.rotationDegrees?.z));
  return {
    index: command.index,
    label: command.label,
    type: classifyWindow(command),
    startMs: command.startMs,
    endMs: command.endMs,
    durationMs: command.durationMs,
    dx: command.dx,
    dy: command.dy,
    keys: command.keys,
    sampleCount: samples.length,
    view: span(view),
    viewPitch: span(viewPitch),
    rotX: span(rotX),
    rotY: span(rotY),
    rotZ: span(rotZ),
  };
});

const summary = {
  macro: path.basename(macroPath),
  transforms: path.basename(transformsPath),
  replayStartSeconds,
  trackLabel: track.label,
  playerNameGuess: track.playerNameGuess,
  windows,
};

console.log(JSON.stringify(summary, null, 2));
