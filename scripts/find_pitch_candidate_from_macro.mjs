import fs from "fs";
import path from "path";

if (process.argv.length < 4) {
  console.error("Usage: node scripts/find_pitch_candidate_from_macro.mjs <macro.txt> <transforms.json>");
  process.exit(1);
}

const macroPath = process.argv[2];
const transformsPath = process.argv[3];

const macro = JSON.parse(fs.readFileSync(macroPath, "utf8"));
const transforms = JSON.parse(fs.readFileSync(transformsPath, "utf8"));
const track = (transforms.primaryTracks && transforms.primaryTracks[0]) || (transforms.tracks && transforms.tracks[0]);

if (!track || !Array.isArray(track.samples) || track.samples.length === 0) {
  throw new Error("No samples found in transform export.");
}

const firstTimed = track.samples.find((sample) => typeof sample.timeInSeconds === "number");
if (!firstTimed) {
  throw new Error("Transform export has no replay clock.");
}

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
  return commands.map((command, index) => {
    const startMs = parseMs(command.at ?? 0);
    const durationMs = parseMs(command.duration ?? 0);
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
      startMs,
      endMs: startMs + durationMs,
      durationMs,
      dx,
      dy,
      keys,
    };
  });
}

function sampleElapsedMs(sample) {
  return (firstTimed.timeInSeconds - sample.timeInSeconds) * 1000;
}

function wrapDegrees(value) {
  while (value > 180) value -= 360;
  while (value < -180) value += 360;
  return value;
}

function unwrap(values) {
  if (values.length === 0) return [];
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

function pitchFold(value) {
  let current = wrapDegrees(value);
  while (current > 90) current = 180 - current;
  while (current < -90) current = -180 - current;
  return current;
}

function slope(values) {
  if (values.length < 2) return 0;
  return values[values.length - 1] - values[0];
}

function span(values) {
  if (values.length === 0) return 0;
  return Math.max(...values) - Math.min(...values);
}

function average(values) {
  if (!values.length) return 0;
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}

const commands = normalizeCommands(macro.commands);
const yawOnly = commands.filter((command) => command.dx !== 0 && command.dy === 0 && command.keys.length === 0);
const pitchOnly = commands.filter((command) => command.dy !== 0 && command.dx === 0 && command.keys.length === 0);

const candidateExtractors = [
  ["viewPitch", (sample) => sample.viewPitchDegrees],
  ["rotX", (sample) => sample.rotationDegrees?.x],
  ["rotY", (sample) => sample.rotationDegrees?.y],
  ["rotZ", (sample) => sample.rotationDegrees?.z],
  ["pitchFoldX", (sample) => sample.rotationDegrees?.x != null ? pitchFold(sample.rotationDegrees.x) : null],
  ["pitchFoldY", (sample) => sample.rotationDegrees?.y != null ? pitchFold(sample.rotationDegrees.y) : null],
  ["pitchFoldZ", (sample) => sample.rotationDegrees?.z != null ? pitchFold(sample.rotationDegrees.z) : null],
];

function windowValues(command, extractor) {
  return unwrap(
    track.samples
      .filter((sample) => typeof sample.timeInSeconds === "number")
      .filter((sample) => {
        const elapsedMs = sampleElapsedMs(sample);
        return elapsedMs >= command.startMs && elapsedMs <= command.endMs;
      })
      .map(extractor)
      .filter((value) => typeof value === "number"),
  );
}

function sign(value) {
  if (value > 0) return 1;
  if (value < 0) return -1;
  return 0;
}

const scored = candidateExtractors.map(([name, extractor]) => {
  const pitchSpans = [];
  const yawSpans = [];
  let signAgreement = 0;
  let signCount = 0;
  const pitchDetails = [];
  for (const window of pitchOnly) {
    const values = windowValues(window, extractor);
    const currentSpan = span(values);
    const currentSlope = slope(values);
    pitchSpans.push(currentSpan);
    if (Math.abs(currentSlope) > 1) {
      const expected = sign(-window.dy);
      const actual = sign(currentSlope);
      if (expected !== 0 && actual !== 0) {
        signAgreement += expected === actual ? 1 : -1;
        signCount++;
      }
    }
    pitchDetails.push({
      label: window.label,
      samples: values.length,
      span: currentSpan,
      slope: currentSlope,
    });
  }
  for (const window of yawOnly) {
    const values = windowValues(window, extractor);
    yawSpans.push(span(values));
  }
  const avgPitchSpan = average(pitchSpans);
  const avgYawSpan = average(yawSpans);
  const score = avgPitchSpan - (avgYawSpan * 1.5) + (signCount > 0 ? (signAgreement / signCount) * 25 : 0);
  return {
    name,
    score,
    avgPitchSpan,
    avgYawSpan,
    signAgreement: signCount > 0 ? signAgreement / signCount : 0,
    pitchDetails,
  };
}).sort((left, right) => right.score - left.score);

console.log(JSON.stringify({
  macro: path.basename(macroPath),
  transforms: path.basename(transformsPath),
  trackLabel: track.label,
  playerNameGuess: track.playerNameGuess,
  best: scored[0],
  candidates: scored,
}, null, 2));
