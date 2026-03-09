import fs from 'node:fs';
import path from 'node:path';

function usage() {
  console.error('Usage: node scripts/plot_direction_compare.mjs <transforms.json> [output.svg] [actorID-or-label]');
  process.exit(1);
}

if (process.argv.length < 3) {
  usage();
}

const inputPath = process.argv[2];
const outputPath = process.argv[3] && !process.argv[3].startsWith('--')
  ? process.argv[3]
  : inputPath.replace(/\.json$/i, '.direction-compare.svg');
const actorSelector = process.argv[4] || '';

const raw = fs.readFileSync(inputPath, 'utf8');
const data = JSON.parse(raw);
const tracks = Array.isArray(data.primaryTracks) && data.primaryTracks.length
  ? data.primaryTracks
  : (Array.isArray(data.tracks) ? data.tracks : []);
if (!tracks.length) {
  throw new Error('No tracks found in JSON.');
}

const track = pickTrack(tracks, actorSelector);
const graph = buildDirectionGraph(track);
if (!graph.points.length) {
  throw new Error('No comparable movement/viewing samples found.');
}

const svg = renderSvg(track, graph, data);
fs.writeFileSync(outputPath, svg, 'utf8');

const summary = graph.summary;
process.stdout.write(JSON.stringify({
  input: inputPath,
  output: outputPath,
  actorID: track.actorID,
  label: track.label || '',
  playerNameGuess: track.playerNameGuess || '',
  movementSamples: summary.stepSamples,
  plottedSamples: summary.plottedSamples,
  meanErrorDegrees: round(summary.meanErrorDegrees, 3),
  medianErrorDegrees: round(summary.medianErrorDegrees, 3),
  p90ErrorDegrees: round(summary.p90ErrorDegrees, 3),
  meanCosineAgreement: round(summary.meanCosineAgreement, 6),
}, null, 2));

function pickTrack(tracks, selector) {
  if (!selector) {
    return tracks[0];
  }
  const lowered = selector.toLowerCase();
  return tracks.find((track) => {
    return [
      track.actorID,
      track.label,
      track.playerNameGuess,
      `${track.playerNameGuess || ''} (${track.label || ''})`,
    ].filter(Boolean).some((value) => String(value).toLowerCase() === lowered);
  }) || tracks[0];
}

function buildDirectionGraph(track) {
  const positionPoints = [];
  for (let sampleIndex = 0; sampleIndex < track.samples.length; sampleIndex += 1) {
    const sample = track.samples[sampleIndex];
    if (sample.changed !== 'position' || !sample.position || sample.viewingDirectionDegrees == null) {
      continue;
    }
    positionPoints.push({
      sampleIndex,
      offset: sample.offset ?? sampleIndex,
      time: sample.time || '',
      position: sample.position,
      viewingDegrees: Number(sample.viewingDirectionDegrees),
    });
  }
  if (positionPoints.length < 2) {
    return { points: [], summary: { plottedSamples: 0, stepSamples: 0 } };
  }
  const gap = positionPoints.length <= 4 ? 1 : 4;
  const points = [];
  for (let index = gap; index < positionPoints.length; index += 1) {
    const previous = positionPoints[index - gap];
    const current = positionPoints[index];
    const dx = Number(current.position.x || 0) - Number(previous.position.x || 0);
    const dy = Number(current.position.y || 0) - Number(previous.position.y || 0);
    if (Math.hypot(dx, dy) < 0.15) {
      continue;
    }
    points.push({
      sampleIndex: current.sampleIndex,
      offset: current.offset,
      time: current.time,
      movementDegrees: degrees(Math.atan2(dy, dx)),
      viewingDegrees: Number(current.viewingDegrees),
    });
  }
  const unwrapped = unwrapFields(points, ['movementDegrees', 'viewingDegrees']).map((point) => ({
    ...point,
    deltaDegrees: wrapDegrees(point.movementDegrees - point.viewingDegrees),
  }));
  return {
    points: unwrapped,
    summary: summarize(unwrapped),
  };
}

function summarize(points) {
  const errors = points.map((point) => Math.abs(wrapDegrees(point.movementDegrees - point.viewingDegrees)));
  const sorted = [...errors].sort((a, b) => a - b);
  const cosineSum = points.reduce((sum, point) => sum + Math.cos(wrapDegrees(point.movementDegrees - point.viewingDegrees) * Math.PI / 180), 0);
  return {
    plottedSamples: points.length,
    stepSamples: points.length,
    meanErrorDegrees: mean(errors),
    medianErrorDegrees: percentile(sorted, 0.5),
    p90ErrorDegrees: percentile(sorted, 0.9),
    meanCosineAgreement: cosineSum / Math.max(points.length, 1),
  };
}

function renderSvg(track, graph, data) {
  const width = 1400;
  const height = 720;
  const padLeft = 60;
  const padRight = 24;
  const padTop = 54;
  const padBottom = 46;
  const plotWidth = width - padLeft - padRight;
  const plotHeight = height - padTop - padBottom;
  const minValue = Math.min(...graph.points.flatMap((point) => [point.movementDegrees, point.viewingDegrees]));
  const maxValue = Math.max(...graph.points.flatMap((point) => [point.movementDegrees, point.viewingDegrees]));
  const yMin = minValue === maxValue ? minValue - 1 : minValue;
  const yMax = minValue === maxValue ? maxValue + 1 : maxValue;
  const xAt = (index) => padLeft + (plotWidth * (graph.points.length <= 1 ? 0 : index / (graph.points.length - 1)));
  const yAt = (value) => padTop + ((yMax - value) / Math.max(yMax - yMin, 0.0001)) * plotHeight;
  const polyline = (field) => graph.points.map((point, index) => `${xAt(index)},${yAt(point[field])}`).join(' ');
  const grid = [0, 0.25, 0.5, 0.75, 1].map((ratio, index) => {
    const value = yMax - ((yMax - yMin) * ratio);
    const y = padTop + (plotHeight * ratio);
    return `
      <g>
        <line x1="${padLeft}" x2="${width - padRight}" y1="${y}" y2="${y}" stroke="rgba(23,36,34,0.10)" stroke-width="1" />
        <text x="12" y="${y + 4}" fill="rgba(23,36,34,0.58)" font-size="12">${value.toFixed(1)}deg</text>
      </g>
    `;
  }).join('\n');
  const title = `${path.basename(inputPath)} :: ${displayTrack(track)}`;
  const subtitle = `samples ${graph.summary.plottedSamples} | mean err ${round(graph.summary.meanErrorDegrees, 3)}deg | cos ${round(graph.summary.meanCosineAgreement, 6)}`;
  const source = data?.directionSearch?.bestFusedCandidate?.source || 'unknown';
  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${width} ${height}" width="${width}" height="${height}">
  <rect width="${width}" height="${height}" fill="#fffaf4"/>
  <text x="${padLeft}" y="24" fill="#172422" font-size="24" font-family="Consolas, monospace" font-weight="700">${escapeXml(title)}</text>
  <text x="${padLeft}" y="44" fill="#5a6a66" font-size="14" font-family="Consolas, monospace">${escapeXml(subtitle)} | source ${escapeXml(source)}</text>
  ${grid}
  <line x1="${padLeft}" x2="${padLeft}" y1="${padTop}" y2="${height - padBottom}" stroke="rgba(23,36,34,0.22)" stroke-width="1.5" />
  <line x1="${padLeft}" x2="${width - padRight}" y1="${height - padBottom}" y2="${height - padBottom}" stroke="rgba(23,36,34,0.22)" stroke-width="1.5" />
  <polyline fill="none" stroke="#c84c2a" stroke-width="3" points="${polyline('movementDegrees')}" />
  <polyline fill="none" stroke="#2f7f6b" stroke-width="3" stroke-opacity="0.88" points="${polyline('viewingDegrees')}" />
  <text x="${width - 280}" y="24" fill="#c84c2a" font-size="14" font-family="Consolas, monospace">movement heading</text>
  <text x="${width - 280}" y="44" fill="#2f7f6b" font-size="14" font-family="Consolas, monospace">view heading</text>
</svg>`;
}

function displayTrack(track) {
  return track.playerNameGuess ? `${track.playerNameGuess} (${track.label || track.actorID})` : (track.label || track.actorID);
}

function mean(values) {
  if (!values.length) return 0;
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}

function percentile(sortedValues, fraction) {
  if (!sortedValues.length) return 0;
  const index = Math.min(sortedValues.length - 1, Math.max(0, Math.floor((sortedValues.length - 1) * fraction)));
  return sortedValues[index];
}

function unwrapFields(points, fields) {
  const series = Object.fromEntries(fields.map((field) => [field, unwrap(points.map((point) => point[field]))]));
  return points.map((point, index) => ({
    ...point,
    ...Object.fromEntries(fields.map((field) => [field, series[field][index]])),
  }));
}

function unwrap(values) {
  if (!values.length) return values;
  const out = [values[0]];
  let offset = 0;
  let previous = values[0];
  for (let index = 1; index < values.length; index += 1) {
    const value = values[index];
    const delta = value - previous;
    if (delta > 180) {
      offset -= 360;
    } else if (delta < -180) {
      offset += 360;
    }
    out.push(value + offset);
    previous = value;
  }
  return out;
}

function degrees(radians) {
  return radians * 180 / Math.PI;
}

function wrapDegrees(value) {
  let out = value;
  while (out <= -180) out += 360;
  while (out > 180) out -= 360;
  return out;
}

function round(value, digits) {
  const factor = 10 ** digits;
  return Math.round(value * factor) / factor;
}

function escapeXml(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&apos;');
}
