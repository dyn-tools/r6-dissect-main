import React, { useCallback, useDeferredValue, useEffect, useMemo, useState } from 'https://esm.sh/react@18.3.1';
import { createRoot } from 'https://esm.sh/react-dom@18.3.1/client';
import htm from 'https://esm.sh/htm@3.1.1';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Checkbox,
  Chip,
  CssBaseline,
  Divider,
  FormControl,
  InputLabel,
  LinearProgress,
  MenuItem,
  Paper,
  Select,
  Slider,
  Stack,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from 'https://esm.sh/@mui/material@5.16.7?bundle&deps=react@18.3.1,react-dom@18.3.1';
import { ThemeProvider, alpha, createTheme } from 'https://esm.sh/@mui/material@5.16.7/styles?bundle&deps=react@18.3.1,react-dom@18.3.1';

const html = htm.bind(React.createElement);
const DEFAULT_PATH = new URLSearchParams(window.location.search).get('data') || '/replays/Match-2026-02-15_18-28-03-10012-R01.transforms.json';
const DEFAULT_LIBRARY_ROOT = 'replays';
const DEFAULT_SELECTION_SIZE = 8;
const DEFAULT_TRAIL_LENGTH = 120;
const SVG_WIDTH = 1200;
const SVG_HEIGHT = 760;
const PADDING = 48;
const PLAY_INTERVAL_MS = 140;
const CONVERSION_POLL_MS = 1000;
const START_TRANSITION = React.startTransition || ((fn) => fn());
const VIEW_PLANES = {
  xy: { id: 'xy', label: 'XY', horizontal: 'x', vertical: 'y', normal: 'z' },
  xz: { id: 'xz', label: 'XZ', horizontal: 'x', vertical: 'z', normal: 'y' },
  yz: { id: 'yz', label: 'YZ', horizontal: 'y', vertical: 'z', normal: 'x' },
};
const AXIS_LABELS = { x: 'X', y: 'Y', z: 'Z' };
const EMPTY_LIBRARY = Object.freeze({
  replayFiles: [],
  replayFolders: [],
  transformFiles: [],
  probeFiles: [],
  cameraFiles: [],
  otherJsonFiles: [],
});
const theme = createTheme({
  palette: {
    mode: 'light',
    primary: { main: '#9f4c28' },
    secondary: { main: '#355c5f' },
    background: {
      default: '#efe4d6',
      paper: 'rgba(255, 250, 244, 0.82)',
    },
    text: {
      primary: '#172422',
      secondary: '#5a6a66',
    },
  },
  shape: { borderRadius: 18 },
  typography: {
    fontFamily: 'Bahnschrift, Aptos, "Segoe UI Variable Display", sans-serif',
    h3: { fontWeight: 700 },
    h4: { fontWeight: 700 },
    h5: { fontWeight: 700 },
    h6: { fontWeight: 700 },
    overline: { letterSpacing: '0.12em', fontWeight: 700 },
  },
  components: {
    MuiCssBaseline: {
      styleOverrides: {
        body: {
          minHeight: '100vh',
          background: [
            'radial-gradient(circle at top left, rgba(159, 76, 40, 0.22), transparent 24%)',
            'radial-gradient(circle at bottom right, rgba(23, 36, 34, 0.16), transparent 28%)',
            'linear-gradient(135deg, #efe4d6, #d9cab7)',
          ].join(','),
        },
        '#root': {
          minHeight: '100vh',
        },
      },
    },
    MuiPaper: {
      styleOverrides: {
        root: {
          backdropFilter: 'blur(14px)',
        },
      },
    },
  },
});
const ActorList = React.memo(function ActorList({ tracks, selectedTrackIds, focusActorID, onToggleActor, onChooseFocusActor }) {
  return html`
    <${Box}
      sx=${{
        minHeight: 220,
        flex: 1,
        overflowY: 'auto',
        pr: 0.75,
        display: 'flex',
        flexDirection: 'column',
        gap: 1,
      }}
    >
      ${tracks.map((track) => {
        const selected = selectedTrackIds.includes(track.actorID);
        const centered = focusActorID === track.actorID;
        const color = colorForActor(track.actorID);
        return html`
          <${Box}
            key=${track.actorID}
            sx=${{
              display: 'grid',
              gridTemplateColumns: 'auto minmax(0, 1fr) auto',
              gap: 1,
              alignItems: 'start',
              p: 1.25,
              borderRadius: 2.5,
              border: '1px solid',
              borderColor: selected ? alpha(theme.palette.primary.main, 0.4) : alpha(theme.palette.text.primary, 0.1),
              bgcolor: selected ? alpha(theme.palette.primary.main, 0.08) : alpha('#ffffff', 0.45),
            }}
          >
            <${Checkbox} size="small" checked=${selected} onChange=${() => onToggleActor(track.actorID)} />
            <${Box} sx=${{ minWidth: 0 }}>
              <${Stack} direction="row" spacing=${1} alignItems="center" flexWrap="wrap" useFlexGap=${true}>
                <${Typography} variant="body2" sx=${{ fontFamily: 'Consolas, "Cascadia Mono", monospace', fontWeight: 700 }}>
                  ${displayTrack(track)}
                </${Typography}>
                ${centered ? html`<${Chip} size="small" color="primary" label="Centered" />` : null}
              </${Stack}>
              <${Typography} variant="caption" color="text.secondary">
                ${track.teamRoleGuess ? `${track.teamRoleGuess} / ` : ``}${track.operatorGuess?.name ? `${track.operatorGuess.name} / ` : ``}${track.playerGuessSource ? `${track.playerGuessSource} / ` : ``}${shortActor(track.actorID)} / ${track.positionSamples} pos / ${track.rotationSamples} rot / ${track.samples.length} merged
              </${Typography}>
            </${Box}>
            <${Stack} spacing=${0.75} alignItems="flex-end">
              <${Button} size="small" variant=${centered ? 'contained' : 'outlined'} onClick=${() => onChooseFocusActor(track.actorID)}>
                ${centered ? 'Centered' : 'Center'}
              </${Button}>
              <${Box} sx=${{ width: 12, height: 12, borderRadius: '999px', bgcolor: color }} />
            </${Stack}>
          </${Box}>
        `;
      })}
    </${Box}>
  `;
});
function App() {
  const [libraryRoot, setLibraryRoot] = useState(DEFAULT_LIBRARY_ROOT);
  const [library, setLibrary] = useState(EMPTY_LIBRARY);
  const [selectedReplayInput, setSelectedReplayInput] = useState('');
  const [selectedFolderInput, setSelectedFolderInput] = useState('');
  const [pathInput, setPathInput] = useState(DEFAULT_PATH);
  const [sourceLabel, setSourceLabel] = useState(DEFAULT_PATH);
  const [data, setData] = useState(null);
  const [selectedActors, setSelectedActors] = useState([]);
  const [focusActorID, setFocusActorID] = useState('');
  const [cursor, setCursor] = useState(0);
  const [trailLength, setTrailLength] = useState(DEFAULT_TRAIL_LENGTH);
  const [viewPlane, setViewPlane] = useState('xy');
  const [rotationAxis, setRotationAxis] = useState('z');
  const [denseProbeGroupKey, setDenseProbeGroupKey] = useState('');
  const [isPlaying, setIsPlaying] = useState(false);
  const [loading, setLoading] = useState(false);
  const [libraryLoading, setLibraryLoading] = useState(false);
  const [conversionState, setConversionState] = useState(null);
  const [activeConversionKey, setActiveConversionKey] = useState('');
  const [error, setError] = useState('');
  const deferredCursor = useDeferredValue(cursor);

  useEffect(() => {
    loadLibrary(DEFAULT_LIBRARY_ROOT);
  }, []);

  useEffect(() => {
    loadFromPath(DEFAULT_PATH);
  }, []);

  useEffect(() => {
    if (!isPlaying || !data) {
      return undefined;
    }
    const maxCursor = maxTrackLength(data.tracks) - 1;
    if (maxCursor <= 0) {
      return undefined;
    }
    const handle = window.setInterval(() => {
      START_TRANSITION(() => {
        setCursor((current) => (current >= maxCursor ? 0 : current + 1));
      });
    }, PLAY_INTERVAL_MS);
    return () => window.clearInterval(handle);
  }, [data, isPlaying]);

  const tracks = data?.primaryTracks?.length ? data.primaryTracks : (data?.tracks || []);
  const players = data?.header?.players || [];
  const selectedTrackIds = useMemo(() => {
    if (tracks.length === 0) {
      return [];
    }
    if (selectedActors.length > 0) {
      return selectedActors.filter((actorID) => tracks.some((track) => track.actorID === actorID));
    }
    return tracks.slice(0, DEFAULT_SELECTION_SIZE).map((track) => track.actorID);
  }, [selectedActors, tracks]);
  const selectedTracks = useMemo(
    () => tracks.filter((track) => selectedTrackIds.includes(track.actorID)),
    [tracks, selectedTrackIds],
  );

  useEffect(() => {
    if (tracks.length === 0) {
      if (focusActorID) {
        setFocusActorID('');
      }
      return;
    }
    if (selectedTrackIds.includes(focusActorID)) {
      return;
    }
    const fallback = selectedTrackIds[0] || tracks[0]?.actorID || '';
    if (fallback && fallback !== focusActorID) {
      setFocusActorID(fallback);
    }
  }, [focusActorID, selectedTrackIds, tracks]);

  const focusTrack = useMemo(
    () => tracks.find((track) => track.actorID === focusActorID) || selectedTracks[0] || tracks[0] || null,
    [focusActorID, selectedTracks, tracks],
  );
  const scene = useMemo(
    () => buildScene(data, selectedTracks, focusTrack, deferredCursor, trailLength, viewPlane, rotationAxis),
    [data, selectedTracks, focusTrack, deferredCursor, trailLength, viewPlane, rotationAxis],
  );
  const maxCursorValue = Math.max(0, maxTrackLength(tracks) - 1);
  const hasTimedSamples = useMemo(
    () => tracks.some((track) => track.samples.some((sample) => sample.time)),
    [tracks],
  );
  const currentFocusTime = useMemo(
    () => sampleTimeAtCursor(focusTrack, deferredCursor),
    [focusTrack, deferredCursor],
  );
  const currentSummaries = useMemo(
    () => scene.entries.map((entry) => summarizeTrack(entry, scene.focusPosition, scene.config, rotationAxis)).filter(Boolean),
    [scene, rotationAxis],
  );
  const focusRotationGraph = useMemo(() => buildRotationGraph(focusTrack), [focusTrack]);
  const focusValidation = useMemo(
    () => validationCheckForTrack(data, focusTrack),
    [data, focusTrack],
  );
  const focusDirectionSearch = useMemo(
    () => directionSearchForTrack(data, focusTrack),
    [data, focusTrack],
  );
  const densePropProbe = focusDirectionSearch?.densePropInspection || null;
  const densePropGroups = densePropProbe?.laneGroups || [];
  const selectedDensePropGroup = useMemo(() => {
    if (!densePropGroups.length) {
      return null;
    }
    return densePropGroups.find((group) => probeLaneKey(group) === denseProbeGroupKey) || densePropGroups[0];
  }, [denseProbeGroupKey, densePropGroups]);
  const focusDirectionGraph = useMemo(
    () => buildDirectionGraph(focusTrack, focusValidation, rotationAxis),
    [focusTrack, focusValidation, rotationAxis],
  );
  const transformFiles = library.transformFiles || [];
  const replayFiles = library.replayFiles || [];
  const replayFolders = library.replayFolders || [];
  const cameraFiles = library.cameraFiles || [];
  const probeFiles = library.probeFiles || [];
  const selectedKnownExport = transformFiles.some((file) => file.webPath === pathInput) ? pathInput : '';

  useEffect(() => {
    if (!densePropGroups.length) {
      if (denseProbeGroupKey) {
        setDenseProbeGroupKey('');
      }
      return;
    }
    if (densePropGroups.some((group) => probeLaneKey(group) === denseProbeGroupKey)) {
      return;
    }
    const preferred = densePropGroups.find((group) => group.bestCandidate) || densePropGroups[0];
    setDenseProbeGroupKey(probeLaneKey(preferred));
  }, [denseProbeGroupKey, densePropGroups]);

  async function loadLibrary(requestedRoot = libraryRoot) {
    setLibraryLoading(true);
    try {
      const response = await fetch(`/__viewer/library?dir=${encodeURIComponent(requestedRoot)}`);
      const payload = await response.json();
      if (!response.ok) {
        throw new Error(payload.error || 'Failed to load replay library.');
      }
      setLibrary({
        replayFiles: payload.replayFiles || [],
        replayFolders: payload.replayFolders || [],
        transformFiles: payload.transformFiles || [],
        probeFiles: payload.probeFiles || [],
        cameraFiles: payload.cameraFiles || [],
        otherJsonFiles: payload.otherJsonFiles || [],
      });
      setLibraryRoot(payload.root || requestedRoot);
      setSelectedReplayInput((current) => (
        (payload.replayFiles || []).some((file) => file.path === current)
          ? current
          : (payload.replayFiles || [])[0]?.path || ''
      ));
      setSelectedFolderInput((current) => (
        (payload.replayFolders || []).some((folder) => folder.path === current)
          ? current
          : (payload.replayFolders || [])[0]?.path || ''
      ));
    } catch (loadError) {
      setError(loadError.message);
    } finally {
      setLibraryLoading(false);
    }
  }

  async function loadFromPath(path) {
    setLoading(true);
    setError('');
    try {
      const response = await fetch(path);
      if (!response.ok) {
        throw new Error(`Failed to load ${path}`);
      }
      const json = await response.json();
      hydrateData(json, path);
    } catch (loadError) {
      setError(loadError.message);
    } finally {
      setLoading(false);
    }
  }

  async function runConversion(kind, inputPath, options = {}) {
    if (!inputPath) {
      setError('Pick a replay file or folder first.');
      return;
    }
    const key = `${kind}:${inputPath}`;
    setActiveConversionKey(key);
    setConversionState({
      status: 'running',
      kind,
      inputPath,
    });
    setError('');
    try {
      const response = await fetch('/__viewer/convert', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ kind, inputPath }),
      });
      const payload = await response.json();
      if (!response.ok) {
        throw new Error(payload.error || `Failed to run ${kind}.`);
      }
      let latest = payload;
      setConversionState(latest);
      while (latest?.status === 'running') {
        await sleep(CONVERSION_POLL_MS);
        const statusResponse = await fetch(`/__viewer/convert-status?id=${encodeURIComponent(latest.jobId)}`);
        const statusPayload = await statusResponse.json();
        if (!statusResponse.ok) {
          throw new Error(statusPayload.error || 'Failed to read conversion status.');
        }
        latest = statusPayload;
        setConversionState(latest);
      }
      if (latest?.status !== 'success') {
        throw new Error(latest?.message || `Failed to run ${kind}.`);
      }
      await loadLibrary(libraryRoot);
      const transformOutput = (latest.generatedFiles || []).find((file) => file.kind === 'transform-json' && file.exists);
      if (transformOutput?.webPath) {
        setPathInput(transformOutput.webPath);
      }
      if (options.loadOutput && transformOutput?.webPath) {
        await loadFromPath(transformOutput.webPath);
      }
    } catch (conversionError) {
      setConversionState({
        status: 'error',
        kind,
        inputPath,
        message: conversionError.message,
      });
      setError(conversionError.message);
    } finally {
      setActiveConversionKey('');
    }
  }

  async function handleFileChange(event) {
    const file = event.target.files?.[0];
    if (!file) {
      return;
    }
    setLoading(true);
    setError('');
    try {
      const text = await file.text();
      const json = JSON.parse(text);
      hydrateData(json, file.name);
    } catch (loadError) {
      setError(loadError.message);
    } finally {
      setLoading(false);
      event.target.value = '';
    }
  }

  function hydrateData(json, label) {
    if (!json || !Array.isArray(json.tracks)) {
      throw new Error('Expected a transform export with a top-level "tracks" array.');
    }
    const sortedTracks = [...json.tracks].sort((left, right) => totalSamples(right) - totalSamples(left));
    const nextData = { ...json, tracks: sortedTracks };
    const nextSelection = sortedTracks.slice(0, DEFAULT_SELECTION_SIZE).map((track) => track.actorID);
    START_TRANSITION(() => {
      setData(nextData);
      setSourceLabel(label);
      setCursor(0);
      setIsPlaying(false);
      setSelectedActors(nextSelection);
      setFocusActorID(nextSelection[0] || '');
      if (label.startsWith('/')) {
        setPathInput(label);
        const url = new URL(window.location.href);
        url.searchParams.set('data', label);
        window.history.replaceState({}, '', url);
      }
    });
  }

  const toggleActor = useCallback((actorID) => {
    setSelectedActors((current) => {
      if (current.includes(actorID)) {
        return current.filter((value) => value !== actorID);
      }
      return [...current, actorID];
    });
  }, []);

  const chooseFocusActor = useCallback((actorID) => {
    if (!actorID) {
      return;
    }
    setFocusActorID(actorID);
    setSelectedActors((current) => {
      const base = current.length > 0 ? current : selectedTrackIds;
      return base.includes(actorID) ? base : [actorID, ...base];
    });
  }, [selectedTrackIds]);

  function selectTopTracks() {
    const topTrackIds = tracks.slice(0, DEFAULT_SELECTION_SIZE).map((track) => track.actorID);
    setSelectedActors(topTrackIds);
    if (topTrackIds.length > 0) {
      setFocusActorID(topTrackIds[0]);
    }
  }

  const convertReplayBusy = activeConversionKey === `movement:${selectedReplayInput}`;
  const probeReplayBusy = activeConversionKey === `movement-probe:${selectedReplayInput}`;
  const cameraReplayBusy = activeConversionKey === `blender-camera:${selectedReplayInput}`;
  const convertFolderBusy = activeConversionKey === `movement:${selectedFolderInput}`;

  return html`
    <${ThemeProvider} theme=${theme}>
      <${CssBaseline} />
      <${Box}
        sx=${{
          minHeight: '100vh',
          p: { xs: 1.5, md: 2 },
          display: 'grid',
          gridTemplateColumns: { xs: '1fr', xl: '360px minmax(0, 1fr)' },
          gap: 2,
        }}
      >
        <${Paper}
          elevation=${10}
          sx=${{
            p: 2.5,
            display: 'flex',
            flexDirection: 'column',
            gap: 2,
            minWidth: 0,
            minHeight: { xl: 'calc(100vh - 32px)' },
            maxHeight: { xl: 'calc(100vh - 32px)' },
            position: { xl: 'sticky' },
            top: { xl: 16 },
            overflow: 'hidden',
            bgcolor: alpha('#fffaf4', 0.74),
            border: '1px solid rgba(255,255,255,0.42)',
            boxShadow: '0 22px 60px rgba(45, 31, 20, 0.18)',
          }}
        >
          <${Stack} spacing=${0.75}>
            <${Typography} variant="overline" color="text.secondary">r6-dissect</${Typography}>
            <${Typography} variant="h4" sx=${{ lineHeight: 0.95, textTransform: 'uppercase', letterSpacing: '0.08em' }}>
              Movement Viewer
            </${Typography}>
            <${Typography} variant="body2" color="text.secondary">
              Local control room for replay browsing, conversion, and track validation without dropping back to PowerShell.
            </${Typography}>
          </${Stack}>

          <${Paper} variant="outlined" sx=${{ p: 1.5, bgcolor: alpha('#fff', 0.56), borderColor: 'rgba(77, 52, 34, 0.16)' }}>
            <${Stack} spacing=${1.25}>
              <${Stack} direction="row" alignItems="center" justifyContent="space-between" spacing=${1}>
                <${Typography} variant="overline" color="text.secondary">Control Room</${Typography}>
                <${Chip}
                  size="small"
                  color="primary"
                  variant="outlined"
                  label=${`${replayFiles.length} rounds / ${transformFiles.length} transforms`}
                />
              </${Stack}>
              <${TextField}
                label="Library Root"
                value=${libraryRoot}
                onChange=${(event) => setLibraryRoot(event.target.value)}
                size="small"
                fullWidth=${true}
                placeholder="replays"
              />
              <${Stack} direction="row" spacing=${1} flexWrap="wrap" useFlexGap=${true}>
                <${Button} variant="contained" onClick=${() => loadLibrary(libraryRoot)} disabled=${libraryLoading}>
                  ${libraryLoading ? 'Refreshing...' : 'Refresh Library'}
                </${Button}>
                <${Chip} size="small" label=${`${replayFolders.length} folders`} variant="outlined" />
                <${Chip} size="small" label=${`${probeFiles.length} probes`} variant="outlined" />
                <${Chip} size="small" label=${`${cameraFiles.length} cameras`} variant="outlined" />
              </${Stack}>

              <${FormControl} size="small" fullWidth=${true}>
                <${InputLabel} id="replay-round-label">Replay Round</${InputLabel}>
                <${Select}
                  labelId="replay-round-label"
                  label="Replay Round"
                  value=${selectedReplayInput}
                  onChange=${(event) => setSelectedReplayInput(event.target.value)}
                >
                  <${MenuItem} value="">
                    <em>Choose a replay round</em>
                  </${MenuItem}>
                  ${replayFiles.map((file) => html`<${MenuItem} key=${file.path} value=${file.path}>${file.path}</${MenuItem}>`)}
                </${Select}>
              </${FormControl}>

              <${Stack} direction="row" spacing=${1} flexWrap="wrap" useFlexGap=${true}>
                <${Button}
                  variant="contained"
                  onClick=${() => runConversion('movement', selectedReplayInput)}
                  disabled=${!selectedReplayInput || convertReplayBusy}
                >
                  ${convertReplayBusy ? 'Converting...' : 'Convert Round'}
                </${Button}>
                <${Button}
                  variant="outlined"
                  onClick=${() => runConversion('movement', selectedReplayInput, { loadOutput: true })}
                  disabled=${!selectedReplayInput || convertReplayBusy}
                >
                  Convert + Load
                </${Button}>
                <${Button}
                  variant="outlined"
                  onClick=${() => runConversion('movement-probe', selectedReplayInput)}
                  disabled=${!selectedReplayInput || probeReplayBusy}
                >
                  ${probeReplayBusy ? 'Probing...' : 'Probe'}
                </${Button}>
                <${Button}
                  variant="outlined"
                  onClick=${() => runConversion('blender-camera', selectedReplayInput)}
                  disabled=${!selectedReplayInput || cameraReplayBusy}
                >
                  ${cameraReplayBusy ? 'Exporting...' : 'Camera Script'}
                </${Button}>
              </${Stack}>

              <${FormControl} size="small" fullWidth=${true}>
                <${InputLabel} id="replay-folder-label">Replay Folder</${InputLabel}>
                <${Select}
                  labelId="replay-folder-label"
                  label="Replay Folder"
                  value=${selectedFolderInput}
                  onChange=${(event) => setSelectedFolderInput(event.target.value)}
                >
                  <${MenuItem} value="">
                    <em>Choose a replay folder</em>
                  </${MenuItem}>
                  ${replayFolders.map((folder) => html`
                    <${MenuItem} key=${folder.path} value=${folder.path}>
                      ${folder.path} (${folder.replayCount} rounds)
                    </${MenuItem}>
                  `)}
                </${Select}>
              </${FormControl}>

              <${Stack} direction="row" spacing=${1} flexWrap="wrap" useFlexGap=${true}>
                <${Button}
                  variant="outlined"
                  onClick=${() => runConversion('movement', selectedFolderInput)}
                  disabled=${!selectedFolderInput || convertFolderBusy}
                >
                  ${convertFolderBusy ? 'Batch Converting...' : 'Batch Convert Folder'}
                </${Button}>
              </${Stack}>

              <${Typography} variant="caption" color="text.secondary">
                Round conversions generate transform JSON sidecars, probe runs generate probe JSON sidecars, and camera export writes a Blender Python sidecar next to the replay unless you override it from the CLI.
              </${Typography}>

              ${conversionState ? renderConversionStatus(conversionState, loadFromPath) : null}
            </${Stack}>
          </${Paper}>

          <${Stack} spacing=${1.5}>
            <${TextField}
              label="Replay Export Path"
              value=${pathInput}
              onChange=${(event) => setPathInput(event.target.value)}
              size="small"
              placeholder="/replays/your-export.transforms.json"
              fullWidth=${true}
            />
            <${Stack} direction="row" spacing=${1} flexWrap="wrap">
              <${Button} variant="contained" onClick=${() => loadFromPath(pathInput)} disabled=${loading}>
                ${loading ? 'Loading...' : 'Load Path'}
              </${Button}>
              <${Button} variant="outlined" onClick=${selectTopTracks} disabled=${tracks.length === 0}>
                Top ${DEFAULT_SELECTION_SIZE}
              </${Button}>
              <${Button} variant="outlined" component="label">
                Open JSON
                <input hidden type="file" accept=".json,application/json" onChange=${handleFileChange} />
              </${Button}>
            </${Stack}>
          </${Stack}>

          <${FormControl} size="small" fullWidth=${true}>
            <${InputLabel} id="known-files-label">Known Transform Exports</${InputLabel}>
            <${Select}
              labelId="known-files-label"
              label="Known Transform Exports"
              value=${selectedKnownExport}
              onChange=${(event) => setPathInput(event.target.value)}
            >
              <${MenuItem} value="">
                <em>Choose a local JSON export</em>
              </${MenuItem}>
              ${transformFiles.map((file) => html`<${MenuItem} key=${file.path} value=${file.webPath}>${file.path}</${MenuItem}>`)}
            </${Select}>
          </${FormControl}>

          <${FormControl} size="small" fullWidth=${true} disabled=${tracks.length === 0}>
            <${InputLabel} id="focus-actor-label">Focus Actor</${InputLabel}>
            <${Select}
              labelId="focus-actor-label"
              label="Focus Actor"
              value=${focusTrack?.actorID || ''}
              onChange=${(event) => chooseFocusActor(event.target.value)}
            >
              ${tracks.map((track) => html`
                <${MenuItem} key=${track.actorID} value=${track.actorID}>
                  ${displayTrack(track)} · ${track.teamRoleGuess ? `${track.teamRoleGuess} / ` : ``}${shortActor(track.actorID)} · ${track.samples.length} samples
                </${MenuItem}>
              `)}
            </${Select}>
          </${FormControl}>

          <${Typography} variant="caption" color="text.secondary">
            Player names are heuristic guesses from death order and kill proximity. Raw actor IDs stay visible below each entry for validation.
          </${Typography}>

          <${Stack} spacing=${1.25}>
            <${Box}>
              <${Typography} variant="overline" color="text.secondary">View Plane</${Typography}>
              <${ToggleButtonGroup}
                exclusive=${true}
                value=${viewPlane}
                size="small"
                onChange=${(_, nextValue) => {
                  if (nextValue) {
                    setViewPlane(nextValue);
                  }
                }}
                sx=${{ mt: 0.75, flexWrap: 'wrap' }}
              >
                ${Object.values(VIEW_PLANES).map((plane) => html`<${ToggleButton} key=${plane.id} value=${plane.id}>${plane.label}</${ToggleButton}>`)}
              </${ToggleButtonGroup}>
            </${Box}>
            <${Box}>
              <${Typography} variant="overline" color="text.secondary">Facing Axis</${Typography}>
              <${ToggleButtonGroup}
                exclusive=${true}
                value=${rotationAxis}
                size="small"
                onChange=${(_, nextValue) => {
                  if (nextValue) {
                    setRotationAxis(nextValue);
                  }
                }}
                sx=${{ mt: 0.75, flexWrap: 'wrap' }}
              >
                ${['x', 'y', 'z'].map((axis) => html`<${ToggleButton} key=${axis} value=${axis}>${AXIS_LABELS[axis]}</${ToggleButton}>`)}
              </${ToggleButtonGroup}>
            </${Box}>
          </${Stack}>

          ${error ? html`<${Alert} severity="error">${error}</${Alert}>` : null}
          ${!hasTimedSamples && data ? html`<${Alert} severity="info">No reliable replay clock is attached yet. The scrubber still uses sample order, not wall-clock time.</${Alert}>` : null}
          ${data?.positionPropID || data?.rotationPropID ? html`
            <${Paper} variant="outlined" sx=${{ p: 1.25, bgcolor: alpha('#fff', 0.56), borderColor: 'rgba(77, 52, 34, 0.16)' }}>
              <${Stack} spacing=${1}>
                <${Typography} variant="overline" color="text.secondary">Prop Discovery</${Typography}>
                <${Typography} variant="body2">
                  Position ${shortProp(data.positionPropID)} | Rotation ${shortProp(data.rotationPropID)}
                </${Typography}>
                ${(data.positionPropCandidates || []).length ? html`
                  <${Box}>
                    <${Typography} variant="caption" color="text.secondary">Position candidates</${Typography}>
                    <${Stack} spacing=${0.5} sx=${{ mt: 0.5 }}>
                      ${(data.positionPropCandidates || []).slice(0, 4).map((candidate) => html`
                        <${Typography} key=${candidate.propID} variant="caption" color=${candidate.selected ? 'text.primary' : 'text.secondary'}>
                          ${candidate.selected ? '>' : '-'} ${shortProp(candidate.propID)} · score ${Math.round(candidate.score)} · abs95 ${Number(candidate.abs95 || 0).toFixed(2)} · strong ${candidate.strongActorCount}
                        </${Typography}>
                      `)}
                    </${Stack}>
                  </${Box}>
                ` : null}
                ${(data.rotationPropCandidates || []).length ? html`
                  <${Box}>
                    <${Typography} variant="caption" color="text.secondary">Rotation candidates</${Typography}>
                    <${Stack} spacing=${0.5} sx=${{ mt: 0.5 }}>
                      ${(data.rotationPropCandidates || []).slice(0, 4).map((candidate) => html`
                        <${Typography} key=${candidate.propID} variant="caption" color=${candidate.selected ? 'text.primary' : 'text.secondary'}>
                          ${candidate.selected ? '>' : '-'} ${shortProp(candidate.propID)} · overlap ${candidate.overlapWithPosition || 0} · score ${Math.round(candidate.score)} · abs95 ${Number(candidate.abs95 || 0).toFixed(2)}
                        </${Typography}>
                      `)}
                    </${Stack}>
                  </${Box}>
                ` : null}
              </${Stack}>
            </${Paper}>
          ` : null}

          <${Divider} flexItem=${true} />

          <${Stack} direction="row" alignItems="center" justifyContent="space-between" spacing=${1}>
            <${Typography} variant="overline" color="text.secondary">Actors</${Typography}>
            <${Chip} size="small" label=${selectedTracks.length + ' visible'} color="primary" variant="outlined" />
          </${Stack}>

          <${ActorList}
            tracks=${tracks}
            selectedTrackIds=${selectedTrackIds}
            focusActorID=${focusTrack?.actorID || ''}
            onToggleActor=${toggleActor}
            onChooseFocusActor=${chooseFocusActor}
          />
        </${Paper}>

        <${Box}
          sx=${{
            minWidth: 0,
            display: 'grid',
            gridTemplateRows: 'auto auto minmax(0, 1fr)',
            gap: 2,
            minHeight: { xl: 'calc(100vh - 32px)' },
          }}
        >
          <${Box}
            sx=${{
              display: 'grid',
              gridTemplateColumns: { xs: '1fr', md: 'repeat(2, minmax(0, 1fr))', xxl: 'repeat(5, minmax(0, 1fr))' },
              gap: 2,
            }}
          >
            ${metricCard('Loaded Export', basename(sourceLabel), data?.header?.map?.name || 'No map yet')}
            ${metricCard('Tracks', String(tracks.length), `${selectedTracks.length} visible / ${players.length} players in round header`)}
            ${metricCard('Cursor', String(deferredCursor), `${maxCursorValue} max sample index${currentFocusTime ? ` / ${currentFocusTime}` : ''}`)}
            ${metricCard('Follow', focusTrack ? displayTrack(focusTrack) : '-', `${scene.config.label} plane / ${AXIS_LABELS[rotationAxis]} heading / ${trailLength} trail`)}
            ${metricCard('Clock', formatClockHeadline(data?.clock), formatClockDetail(data?.clock, currentFocusTime))}
            ${metricCard('Usage', formatUsageHeadline(data?.usage), formatUsageDetail(data?.usage))}
          </${Box}>

          <${Paper}
            elevation=${10}
            sx=${{
              p: 2,
              bgcolor: alpha('#fffaf4', 0.74),
              border: '1px solid rgba(255,255,255,0.42)',
              boxShadow: '0 22px 60px rgba(45, 31, 20, 0.18)',
            }}
          >
            <${Stack} spacing=${1.75}>
              <${Stack} direction="row" spacing=${1} flexWrap="wrap">
                <${Button} variant=${isPlaying ? 'contained' : 'outlined'} onClick=${() => setIsPlaying((value) => !value)} disabled=${maxCursorValue === 0}>
                  ${isPlaying ? 'Pause' : 'Play'}
                </${Button}>
                <${Button} variant="outlined" onClick=${() => setCursor(0)} disabled=${maxCursorValue === 0}>
                  Reset
                </${Button}>
              </${Stack}>

              <${Box}>
                <${Stack} direction="row" justifyContent="space-between" alignItems="center" spacing=${1}>
                  <${Typography} variant="body2" color="text.secondary">Sample Cursor</${Typography}>
                  <${Typography} variant="body2" sx=${{ fontFamily: 'Consolas, "Cascadia Mono", monospace' }}>
                    ${Math.min(deferredCursor, maxCursorValue)}${currentFocusTime ? ` / ${currentFocusTime}` : ''}
                  </${Typography}>
                </${Stack}>
                <${Slider}
                  min=${0}
                  max=${maxCursorValue}
                  value=${Math.min(deferredCursor, maxCursorValue)}
                  onChange=${(_, value) => setCursor(Number(value))}
                  sx=${{ mt: 1 }}
                />
              </${Box}>

              <${Box}>
                <${Stack} direction="row" justifyContent="space-between" alignItems="center" spacing=${1}>
                  <${Typography} variant="body2" color="text.secondary">Trail Length</${Typography}>
                  <${Typography} variant="body2" sx=${{ fontFamily: 'Consolas, "Cascadia Mono", monospace' }}>
                    ${trailLength}
                  </${Typography}>
                </${Stack}>
                <${Slider}
                  min=${10}
                  max=${300}
                  step=${10}
                  value=${trailLength}
                  onChange=${(_, value) => setTrailLength(Number(value))}
                  sx=${{ mt: 1 }}
                />
              </${Box}>
            </${Stack}>
          </${Paper}>

          <${Box}
            sx=${{
              minHeight: 0,
              display: 'grid',
              gridTemplateColumns: { xs: '1fr', xxl: 'minmax(0, 1fr) 340px' },
              gap: 2,
            }}
          >
            <${Paper}
              elevation=${10}
              sx=${{
                p: 2,
                minHeight: 0,
                display: 'flex',
                flexDirection: 'column',
                bgcolor: alpha('#fffaf4', 0.74),
                border: '1px solid rgba(255,255,255,0.42)',
                boxShadow: '0 22px 60px rgba(45, 31, 20, 0.18)',
              }}
            >
              <${Box}
                sx=${{
                  position: 'relative',
                  flex: 1,
                  minHeight: { xs: 420, xl: 0 },
                  borderRadius: 3,
                  overflow: 'hidden',
                  border: '1px solid rgba(23,36,34,0.1)',
                  backgroundImage: [
                    'linear-gradient(180deg, rgba(255,255,255,0.62), rgba(227,216,201,0.78))',
                    'repeating-linear-gradient(0deg, transparent, transparent 27px, rgba(21,34,32,0.05) 28px)',
                    'repeating-linear-gradient(90deg, transparent, transparent 27px, rgba(21,34,32,0.05) 28px)',
                  ].join(','),
                }}
              >
                ${renderSvg(scene, rotationAxis)}
                <${Paper}
                  variant="outlined"
                  sx=${{
                    position: 'absolute',
                    left: 12,
                    bottom: 12,
                    p: 1.2,
                    maxWidth: 380,
                    bgcolor: alpha('#fffaf4', 0.92),
                  }}
                >
                  <${Typography} variant="subtitle2">Follow mode is active.</${Typography}>
                  <${Typography} variant="body2" color="text.secondary">
                    The focus actor stays centered using their latest position at the current sample. The grid is locked to the same frame of reference, so the left list can scroll without moving the viewer.
                  </${Typography}>
                </${Paper}>
              </${Box}>
            </${Paper}>

            <${Paper}
              elevation=${10}
              sx=${{
                p: 2,
                minHeight: 0,
                display: 'flex',
                flexDirection: 'column',
                gap: 2,
                overflow: 'auto',
                bgcolor: alpha('#fffaf4', 0.74),
                border: '1px solid rgba(255,255,255,0.42)',
                boxShadow: '0 22px 60px rgba(45, 31, 20, 0.18)',
              }}
            >
              ${detailCard('Round Header', [
                ['Map', data?.header?.map?.name || '-'],
                ['Site', data?.header?.site || '-'],
                ['Game', data?.header?.gameVersion || '-'],
                ['Match ID', data?.header?.matchID || '-'],
              ])}

              ${detailCard('Reference Frame', [
                ['Focus actor', focusTrack ? displayTrack(focusTrack) : '-'],
                ['Plane', describePlane(scene.config)],
                ['Facing axis', scene.focusHeadingSource || AXIS_LABELS[rotationAxis]],
                ['Focus world pos', scene.focusPosition ? formatVector(scene.focusPosition) : 'No position yet'],
                ['Grid step', scene.gridStep.toFixed(3)],
              ])}

              ${data?.usage ? detailCard('Replay Usage', [
                ['Replay usage', `${formatPercent(data.usage.replayUsagePercent)} (${data.usage.exportedSamples}/${data.usage.totalCandidatePackets})`],
                ['Selected props', `${formatPercent(data.usage.selectedPropUsagePercent)} (${data.usage.exportedSamples}/${data.usage.selectedPropPackets})`],
                ['Primary tracks', `${formatPercent(data.usage.primaryTrackUsagePercent)} (${data.usage.primarySamples}/${data.usage.selectedPropPackets})`],
                ['Primary share', `${formatPercent(data.usage.primarySharePercent)} (${data.usage.primarySamples}/${data.usage.exportedSamples})`],
                ['Candidate actors', `${data.usage.totalCandidateActors || 0} total / ${data.usage.selectedPropActors || 0} selected`],
                ['Selected prop packets', `${data.usage.positionPropPackets || 0} pos / ${data.usage.rotationPropPackets || 0} rot`],
              ]) : null}

              ${data?.clock ? detailCard('Replay Clock', [
                ['Source', data.clock.source || '-'],
                ['Range', formatClockRange(data.clock)],
                ['Entries', `${data.clock.entryCount || 0} entries / ${data.clock.anchorCount || 0} anchors`],
                ['Tick rate', `${Number(data.clock.estimatedTickRate || 0).toFixed(3)} Hz`],
                ['Sample step', `${data.clock.medianStepMilliseconds || 0} ms median / ${data.clock.p90StepMilliseconds || 0} ms p90`],
                ['Duplicates', data.clock.duplicateStepCount || 0],
                ['Trimmed', `${data.clock.trimmedLeadingEntries || 0} lead / ${data.clock.trimmedTrailingEntries || 0} trail`],
                ['Mapping', data.clock.mapping || '-'],
              ]) : null}

              <${Card} variant="outlined" sx=${{ bgcolor: alpha('#ffffff', 0.5) }}>
                <${CardContent}>
                  <${Typography} variant="h6" gutterBottom=${true}>Raw Rotation Packets</${Typography}>
                  ${renderRotationGraph(focusTrack, focusRotationGraph, deferredCursor, rotationAxis)}
                </${CardContent}>
              </${Card}>

              <${Card} variant="outlined" sx=${{ bgcolor: alpha('#ffffff', 0.5) }}>
                <${CardContent}>
                  <${Stack} spacing=${1.5}>
                    <${Typography} variant="h6">Recovered View Heading</${Typography}>
                    ${renderDirectionStatus(focusTrack, focusValidation, focusDirectionSearch, focusDirectionGraph)}
                    ${renderDirectionGraph(focusTrack, focusDirectionGraph, deferredCursor)}
                    ${(focusValidation || focusDirectionSearch) ? html`
                      <${Stack} spacing=${1.25}>
                        ${focusValidation ? html`
                          <${Paper} variant="outlined" sx=${{ p: 1.25, bgcolor: alpha('#fff', 0.56) }}>
                            <${Typography} variant="subtitle2" gutterBottom=${true}>Decoded Export</${Typography}>
                            ${detailRows([
                              ['Source', formatValidationSource(focusValidation)],
                              ['Step samples', focusValidation.stepSamples],
                              ['Offset', formatSignedDegrees(focusValidation.headingOffsetDegrees)],
                              ['Mean error', formatDegrees(focusValidation.meanErrorDegrees)],
                              ['Median error', formatDegrees(focusValidation.medianErrorDegrees)],
                              ['P90 error', formatDegrees(focusValidation.p90ErrorDegrees)],
                              ['Mean cosine', formatMetric(focusValidation.meanCosineAgreement)],
                              ['Makes sense', focusValidation.makesSense ? 'yes' : 'no'],
                            ])}
                          </${Paper}>
                        ` : null}
                        ${focusDirectionSearch ? html`
                          <${Paper} variant="outlined" sx=${{ p: 1.25, bgcolor: alpha('#fff', 0.56) }}>
                            <${Typography} variant="subtitle2" gutterBottom=${true}>Raw Search Anchors</${Typography}>
                            <${Stack} spacing=${1.25}>
                              ${focusDirectionSearch.bestSameActorCandidate ? html`
                                <${Box}>
                                  <${Typography} variant="body2" sx=${{ fontWeight: 700, mb: 0.75 }}>
                                    Same-actor candidate
                                  </${Typography}>
                                  ${detailRows(directionCandidateRows(focusDirectionSearch.bestSameActorCandidate))}
                                </${Box}>
                              ` : null}
                              ${focusDirectionSearch.bestDenseCandidate ? html`
                                <${Box}>
                                  <${Typography} variant="body2" sx=${{ fontWeight: 700, mb: 0.75 }}>
                                    Densest candidate
                                  </${Typography}>
                                  ${detailRows(directionCandidateRows(focusDirectionSearch.bestDenseCandidate))}
                                </${Box}>
                              ` : null}
                            </${Stack}>
                          </${Paper}>
                        ` : null}
                      </${Stack}>
                    ` : html`<${Alert} severity="info">This export does not carry direction-correlation metadata for the current focus actor yet.</${Alert}>`}
                  </${Stack}>
                </${CardContent}>
              </${Card}>

              <${Card} variant="outlined" sx=${{ bgcolor: alpha('#ffffff', 0.5) }}>
                <${CardContent}>
                  <${Stack} spacing=${1.5}>
                    <${Typography} variant="h6">Dense Prop Lanes</${Typography}>
                    ${renderDensePropProbe(
                      densePropProbe,
                      densePropGroups,
                      selectedDensePropGroup,
                      denseProbeGroupKey,
                      setDenseProbeGroupKey,
                    )}
                  </${Stack}>
                </${CardContent}>
              </${Card}>

              <${Card} variant="outlined" sx=${{ bgcolor: alpha('#ffffff', 0.5) }}>
                <${CardContent}>
                  <${Typography} variant="h6" gutterBottom=${true}>Current Samples</${Typography}>
                  ${currentSummaries.length === 0 ? html`<${Alert} severity="info">Select at least one actor with position data.</${Alert}>` : currentSummaries.slice(0, 8).map((summary, index) => html`
                    <${Box} key=${summary.actorID} sx=${{ pt: index === 0 ? 0 : 1.25, mt: index === 0 ? 0 : 1.25, borderTop: index === 0 ? 'none' : '1px solid rgba(23,36,34,0.08)' }}>
                      <${Typography} variant="subtitle2" sx=${{ color: colorForActor(summary.actorID), mb: 1 }}>
                        ${displayTrack(summary)}
                      </${Typography}>
                      ${detailRows([
                        ['Changed', summary.changed],
                        ['Offset', summary.offset],
                        ['World', summary.position],
                        ['Relative', summary.relative],
                        ['Distance', summary.distance],
                        ['Rotation', summary.rotation],
                        ['Heading', summary.heading],
                        ['Heading source', summary.headingSource],
                        ['Time', summary.time],
                      ])}
                    </${Box}>
                  `)}
                </${CardContent}>
              </${Card}>

              <${Card} variant="outlined" sx=${{ bgcolor: alpha('#ffffff', 0.5) }}>
                <${CardContent}>
                  <${Typography} variant="h6" gutterBottom=${true}>Players</${Typography}>
                  <${Alert} severity="info" sx=${{ mb: 2 }}>
                    Round roster comes from the replay header. The focus list on the left shows heuristic actor-to-player guesses, and each entry still includes its raw actor ID for validation.
                  </${Alert}>
                  <${Stack} spacing=${1}>
                    ${players.map((player) => html`
                      <${Stack} key=${player.username} direction="row" justifyContent="space-between" spacing=${1.5}>
                        <${Typography} variant="body2" sx=${{ fontWeight: 700 }}>${player.username}</${Typography}>
                        <${Typography} variant="body2" color="text.secondary">${player.spawn || 'No spawn'}</${Typography}>
                      </${Stack}>
                    `)}
                  </${Stack}>
                </${CardContent}>
              </${Card}>
            </${Paper}>
          </${Box}>
        </${Box}>
      </${Box}>
    </${ThemeProvider}>
  `;
}

function metricCard(label, value, detail) {
  return html`
    <${Card}
      sx=${{
        bgcolor: alpha('#fffaf4', 0.74),
        border: '1px solid rgba(255,255,255,0.42)',
        boxShadow: '0 22px 60px rgba(45, 31, 20, 0.18)',
      }}
    >
      <${CardContent}>
        <${Typography} variant="overline" color="text.secondary">${label}</${Typography}>
        <${Typography} variant="h4" sx=${{ mt: 0.5 }}>${value}</${Typography}>
        <${Typography} variant="body2" color="text.secondary" sx=${{ mt: 1 }}>${detail}</${Typography}>
      </${CardContent}>
    </${Card}>
  `;
}

function detailCard(title, items) {
  return html`
    <${Card} variant="outlined" sx=${{ bgcolor: alpha('#ffffff', 0.5) }}>
      <${CardContent}>
        <${Typography} variant="h6" gutterBottom=${true}>${title}</${Typography}>
        ${detailRows(items)}
      </${CardContent}>
    </${Card}>
  `;
}

function detailRows(items) {
  return html`
    <${Box}
      sx=${{
        display: 'grid',
        gridTemplateColumns: 'auto minmax(0, 1fr)',
        gap: 1,
        alignItems: 'start',
      }}
    >
      ${items.map(([label, value]) => html`
        <${React.Fragment} key=${label}>
          <${Typography} variant="body2" color="text.secondary">${label}</${Typography}>
          <${Typography} variant="body2" sx=${{ fontFamily: 'Consolas, "Cascadia Mono", monospace' }}>
            ${String(value)}
          </${Typography}>
        </${React.Fragment}>
      `)}
    </${Box}>
  `;
}

function renderRotationGraph(track, graph, cursor, rotationAxis) {
  if (!track) {
    return html`<${Alert} severity="info">Choose a focus actor to inspect their rotation stream.</${Alert}>`;
  }
  if (!graph || graph.points.length < 2) {
    return html`<${Alert} severity="info">This track does not have enough rotation samples yet.</${Alert}>`;
  }
  const recoveredCount = track.samples.filter((sample) => sample.viewingDirectionDegrees != null).length;
  const width = 720;
  const height = 220;
  const padLeft = 48;
  const padRight = 18;
  const padTop = 18;
  const padBottom = 28;
  const plotWidth = width - padLeft - padRight;
  const plotHeight = height - padTop - padBottom;
  const cursorPoint = rotationGraphPointAtCursor(graph, cursor);
  const clampedCursor = Math.max(0, Math.min(cursor, graph.maxSampleIndex));
  const xAt = (sampleIndex) => padLeft + (plotWidth * (graph.maxSampleIndex <= 0 ? 0 : sampleIndex / graph.maxSampleIndex));
  const yAt = (value) => padTop + ((graph.maxValue - value) / Math.max(graph.maxValue - graph.minValue, 0.0001)) * plotHeight;
  const polyline = (axis) => graph.points.map((point) => `${xAt(point.sampleIndex)},${yAt(point[axis])}`).join(' ');
  const cursorX = xAt(clampedCursor);
  const legend = [
    { axis: 'x', color: '#c84c2a' },
    { axis: 'y', color: '#2f7f6b' },
    { axis: 'z', color: '#325fc2' },
  ];

  return html`
    <${Stack} spacing=${1.25}>
      <${Typography} variant="body2" color="text.secondary">
        Plotting ${graph.actualPointCount || graph.points.length} actual rotation updates across ${track.samples.length} merged samples. Carried-forward rotation on position packets is skipped so sparse turn data stays visible.
      </${Typography}>
      ${recoveredCount > (graph.actualPointCount || graph.points.length) ? html`
        <${Alert} severity="info">
          This raw packet plot is sparse, but the export carries recovered view heading on ${recoveredCount}/${track.samples.length} merged samples. Use the Recovered View Heading card for the usable full-round look direction.
        </${Alert}>
      ` : null}
      <${Stack} direction="row" spacing=${1} flexWrap="wrap" useFlexGap=${true}>
        ${legend.map((entry) => html`
          <${Chip}
            key=${entry.axis}
            size="small"
            variant=${rotationAxis === entry.axis ? 'filled' : 'outlined'}
            label=${`${AXIS_LABELS[entry.axis]} ${cursorPoint[entry.axis].toFixed(1)}deg`}
            sx=${{
              borderColor: entry.color,
              color: entry.color,
              bgcolor: rotationAxis === entry.axis ? `${entry.color}22` : 'transparent',
            }}
          />
        `)}
      </${Stack}>
      <svg viewBox=${`0 0 ${width} ${height}`} style=${{ width: '100%', height: '220px', display: 'block' }}>
        <rect x="0" y="0" width=${width} height=${height} fill="rgba(255,250,244,0.72)"></rect>
        ${[0, 0.25, 0.5, 0.75, 1].map((ratio, index) => {
          const value = graph.maxValue - (graph.maxValue - graph.minValue) * ratio;
          const y = padTop + plotHeight * ratio;
          return html`
            <g key=${`grid-${index}`}>
              <line x1=${padLeft} x2=${width - padRight} y1=${y} y2=${y} stroke="rgba(23,36,34,0.1)" strokeWidth="1"></line>
              <text x="10" y=${y + 4} fill="rgba(23,36,34,0.55)" font-size="11">${value.toFixed(1)}deg</text>
            </g>
          `;
        })}
        <line x1=${padLeft} x2=${padLeft} y1=${padTop} y2=${height - padBottom} stroke="rgba(23,36,34,0.22)" strokeWidth="1.5"></line>
        <line x1=${padLeft} x2=${width - padRight} y1=${height - padBottom} y2=${height - padBottom} stroke="rgba(23,36,34,0.22)" strokeWidth="1.5"></line>
        ${legend.map((entry) => html`
          <polyline
            key=${entry.axis}
            fill="none"
            stroke=${entry.color}
            strokeWidth=${rotationAxis === entry.axis ? '2.8' : '1.7'}
            strokeOpacity=${rotationAxis === entry.axis ? '1' : '0.72'}
            points=${polyline(entry.axis)}
          ></polyline>
        `)}
        <line x1=${cursorX} x2=${cursorX} y1=${padTop} y2=${height - padBottom} stroke="rgba(23,36,34,0.35)" strokeDasharray="5 4" strokeWidth="1.5"></line>
        ${legend.map((entry) => html`
          <circle
            key=${`${entry.axis}-cursor`}
            cx=${xAt(cursorPoint.sampleIndex)}
            cy=${yAt(cursorPoint[entry.axis])}
            r=${rotationAxis === entry.axis ? '4.5' : '3.2'}
            fill=${entry.color}
          ></circle>
        `)}
        <text x=${width - padRight - 160} y="16" fill="rgba(23,36,34,0.65)" font-size="12">sample ${clampedCursor}</text>
        <text x=${width - padRight - 160} y="32" fill="rgba(23,36,34,0.65)" font-size="12">rot sample ${cursorPoint.sampleIndex}</text>
        <text x=${width - padRight - 160} y="48" fill="rgba(23,36,34,0.65)" font-size="12">range ${graph.minValue.toFixed(1)}..${graph.maxValue.toFixed(1)}deg</text>
        ${cursorPoint.time ? html`
          <text x=${width - padRight - 160} y="64" fill="rgba(23,36,34,0.65)" font-size="12">time ${cursorPoint.time}</text>
        ` : null}
      </svg>
      <${Typography} variant="caption" color="text.secondary">
        Unwrapped degree view of the raw X, Y, and Z rotation packets for ${displayTrack(track)}. Only actual rotation-update packets are plotted here. Recovered look direction lives in the Recovered View Heading panel below when a derived heading stream is available.
      </${Typography}>
    </${Stack}>
  `;
}

function renderDirectionStatus(track, validationCheck, directionSearch, graph) {
  if (!track) {
    return html`<${Alert} severity="info">Choose a focus actor to inspect heading agreement.</${Alert}>`;
  }
  const sameActor = directionSearch?.bestSameActorCandidate || null;
  const sparseCoverage = Boolean(
    graph?.summary?.plottedSamples &&
    graph?.summary?.stepSamples &&
    graph.summary.plottedSamples < graph.summary.stepSamples * 0.25,
  );
  if (validationCheck?.makesSense) {
    if (sparseCoverage) {
      return html`
        <${Alert} severity="warning">
          The decoded heading has a good anchor on this track, but it only covers a sparse slice of the round. Treat it as a promising lead, not a solved full-round camera.
        </${Alert}>
      `;
    }
    if (directionSearch?.bestFusedCandidate?.source === 'movement-smoothed-fallback') {
      return html`
        <${Alert} severity="success">
          The recovered view heading now tracks movement cleanly for ${displayTrack(track)}. On this replay the solved full-round curve is a movement-derived fallback aligned to the good packet anchor, not the sparse raw packet rotation graph above.
        </${Alert}>
      `;
    }
    return html`
      <${Alert} severity="success">
        The decoded export direction already tracks movement well for ${displayTrack(track)}.
      </${Alert}>
    `;
  }
  if (validationCheck && sameActor?.makesSense) {
    return html`
      <${Alert} severity="warning">
        The decoded export does not correlate yet, but the same-actor raw candidate looks promising. Compare the overlaid graph against the raw-anchor summary below.
      </${Alert}>
    `;
  }
  if (validationCheck) {
    return html`
      <${Alert} severity="info">
        The current decoded export does not correlate with movement yet. This usually means the rotation interpretation is still wrong, not that the replay lacks heading data.
      </${Alert}>
    `;
  }
  return html`
    <${Alert} severity="info">
      No decoded heading sanity check is attached to this track yet. The graph falls back to the currently selected rotation axis.
    </${Alert}>
  `;
}

function renderDirectionGraph(track, graph, cursor) {
  if (!track) {
    return html`<${Alert} severity="info">Choose a focus actor to inspect their movement and viewing direction.</${Alert}>`;
  }
  if (!graph || graph.points.length < 2) {
    return html`<${Alert} severity="info">This track does not have enough position and rotation overlap yet to derive a direction graph.</${Alert}>`;
  }
  const width = 720;
  const height = 240;
  const padLeft = 48;
  const padRight = 18;
  const padTop = 18;
  const padBottom = 28;
  const plotWidth = width - padLeft - padRight;
  const plotHeight = height - padTop - padBottom;
  const cursorPoint = directionGraphPointAtCursor(graph, cursor);
  const clampedCursor = Math.max(0, Math.min(cursor, graph.maxSampleIndex));
  const xAt = (sampleIndex) => padLeft + (plotWidth * (graph.maxSampleIndex <= 0 ? 0 : sampleIndex / graph.maxSampleIndex));
  const yAt = (value) => padTop + ((graph.maxValue - value) / Math.max(graph.maxValue - graph.minValue, 0.0001)) * plotHeight;
  const polyline = (field) => graph.points.map((point) => `${xAt(point.sampleIndex)},${yAt(point[field])}`).join(' ');
  const cursorX = xAt(clampedCursor);
  const legend = [
    { key: 'movementDegrees', color: '#c84c2a', label: `Move ${cursorPoint.movementDegrees.toFixed(1)}deg` },
    { key: 'viewingDegrees', color: '#2f7f6b', label: `View ${cursorPoint.viewingDegrees.toFixed(1)}deg` },
    { key: 'deltaDegrees', color: '#325fc2', label: `Delta ${Math.abs(cursorPoint.deltaDegrees).toFixed(1)}deg` },
  ];

  return html`
    <${Stack} spacing=${1.25}>
      <${Typography} variant="body2" color="text.secondary">
        Plotting ${graph.summary.plottedSamples || graph.points.length} usable viewing samples against ${graph.summary.stepSamples || graph.points.length} movement steps using ${graph.summary.sourceLabel || 'the selected heading model'}.
      </${Typography}>
      <${Stack} direction="row" spacing=${1} flexWrap="wrap" useFlexGap=${true}>
        ${legend.map((entry) => html`
          <${Chip}
            key=${entry.key}
            size="small"
            variant="outlined"
            label=${entry.label}
            sx=${{
              borderColor: entry.color,
              color: entry.color,
              bgcolor: `${entry.color}14`,
            }}
          />
        `)}
        <${Chip}
          size="small"
          color=${agreementChipColor(graph.summary)}
          label=${`cos ${formatMetric(graph.summary.meanCosineAgreement)} / mean err ${formatDegrees(graph.summary.meanErrorDegrees)}`}
        />
        ${graph.summary.plottedSamples && graph.summary.stepSamples ? html`
          <${Chip}
            size="small"
            variant="outlined"
            label=${`${graph.summary.plottedSamples}/${graph.summary.stepSamples} plotted`}
          />
        ` : null}
      </${Stack}>
      <svg viewBox=${`0 0 ${width} ${height}`} style=${{ width: '100%', height: '240px', display: 'block' }}>
        <rect x="0" y="0" width=${width} height=${height} fill="rgba(255,250,244,0.72)"></rect>
        ${[0, 0.25, 0.5, 0.75, 1].map((ratio, index) => {
          const value = graph.maxValue - (graph.maxValue - graph.minValue) * ratio;
          const y = padTop + plotHeight * ratio;
          return html`
            <g key=${`dir-grid-${index}`}>
              <line x1=${padLeft} x2=${width - padRight} y1=${y} y2=${y} stroke="rgba(23,36,34,0.1)" strokeWidth="1"></line>
              <text x="10" y=${y + 4} fill="rgba(23,36,34,0.55)" font-size="11">${value.toFixed(1)}deg</text>
            </g>
          `;
        })}
        <line x1=${padLeft} x2=${padLeft} y1=${padTop} y2=${height - padBottom} stroke="rgba(23,36,34,0.22)" strokeWidth="1.5"></line>
        <line x1=${padLeft} x2=${width - padRight} y1=${height - padBottom} y2=${height - padBottom} stroke="rgba(23,36,34,0.22)" strokeWidth="1.5"></line>
        <polyline fill="none" stroke="#c84c2a" strokeWidth="2.8" points=${polyline('movementDegrees')}></polyline>
        <polyline fill="none" stroke="#2f7f6b" strokeWidth="2.8" strokeOpacity="0.88" points=${polyline('viewingDegrees')}></polyline>
        <line x1=${cursorX} x2=${cursorX} y1=${padTop} y2=${height - padBottom} stroke="rgba(23,36,34,0.35)" strokeDasharray="5 4" strokeWidth="1.5"></line>
        <circle cx=${xAt(cursorPoint.sampleIndex)} cy=${yAt(cursorPoint.movementDegrees)} r="4.5" fill="#c84c2a"></circle>
        <circle cx=${xAt(cursorPoint.sampleIndex)} cy=${yAt(cursorPoint.viewingDegrees)} r="4.5" fill="#2f7f6b"></circle>
        <text x=${width - padRight - 180} y="16" fill="rgba(23,36,34,0.65)" font-size="12">sample ${clampedCursor}</text>
        <text x=${width - padRight - 180} y="32" fill="rgba(23,36,34,0.65)" font-size="12">step sample ${cursorPoint.sampleIndex}</text>
        <text x=${width - padRight - 180} y="48" fill="rgba(23,36,34,0.65)" font-size="12">range ${graph.minValue.toFixed(1)}..${graph.maxValue.toFixed(1)}deg</text>
      </svg>
      <${Typography} variant="caption" color="text.secondary">
        Orange is XY movement heading from position deltas. Green is the decoded viewing heading derived with the export's own best sanity-check model${graph.summary.sourceLabel ? ` (${graph.summary.sourceLabel})` : ''}. Both curves are unwrapped so long turns stay continuous.
      </${Typography}>
      ${graph.summary.plottedSamples && graph.summary.stepSamples && graph.summary.plottedSamples < graph.summary.stepSamples * 0.25 ? html`
        <${Alert} severity="warning">
          The selected heading source is sparse on this replay. Good correlation here means the anchor samples line up, not that the full-round camera rotation is solved yet.
        </${Alert}>
      ` : null}
    </${Stack}>
  `;
}

function renderDensePropProbe(probe, groups, selectedGroup, selectedKey, onSelectGroup) {
  if (!probe || !groups.length) {
    return html`<${Alert} severity="info">This export does not include a dense-prop lane inspection for the current focus actor yet.</${Alert}>`;
  }
  return html`
    <${Stack} spacing=${1.25}>
      <${Alert} severity="info">
        Raw lane groups from the densest unresolved prop. This bypasses the decoded main rotation track so you can inspect candidate 0c0c-style lanes directly.
      </${Alert}>
      ${detailRows([
        ['Prop', shortProp(probe.propID)],
        ['Lane groups', groups.length],
        ['Actor envelopes', (probe.actorOrder || []).length],
      ])}
      <${FormControl} size="small" fullWidth=${true}>
        <${InputLabel} id="dense-probe-group-label">Lane Group</${InputLabel}>
        <${Select}
          labelId="dense-probe-group-label"
          label="Lane Group"
          value=${selectedKey}
          onChange=${(event) => onSelectGroup(event.target.value)}
        >
          ${groups.map((group) => html`
            <${MenuItem} key=${probeLaneKey(group)} value=${probeLaneKey(group)}>
              ${`${group.source} / off ${group.vectorOffset} / ${group.totalSamples} samples / ${group.actorCount} actors`}
            </${MenuItem}>
          `)}
        </${Select}>
      </${FormControl}>
      ${selectedGroup ? html`
        ${detailRows([
          ['Source', selectedGroup.source],
          ['Plane', selectedGroup.plane || '-'],
          ['Offset', selectedGroup.vectorOffset],
          ['Samples', selectedGroup.totalSamples],
          ['Actors', selectedGroup.actorCount],
          ['Best candidate', selectedGroup.bestCandidate ? `${formatMetric(selectedGroup.bestCandidate.meanCosineAgreement)} cos / ${formatDegrees(selectedGroup.bestCandidate.meanErrorDegrees)}` : 'none'],
        ])}
        <${Paper} variant="outlined" sx=${{ p: 1.25, bgcolor: alpha('#fff', 0.56) }}>
          <${Typography} variant="subtitle2" gutterBottom=${true}>Lane Actors</${Typography}>
          <${Stack} spacing=${1}>
            ${(selectedGroup.actors || []).map((actor) => html`
              <${Box} key=${`${selectedGroup.source}-${selectedGroup.vectorOffset}-${actor.actorID}`} sx=${{ pb: 1, borderBottom: '1px solid rgba(23,36,34,0.08)' }}>
                ${detailRows([
                  ['Actor', shortActor(actor.actorID)],
                  ['Samples', actor.sampleCount],
                  ['Offsets', `${actor.firstOffset} -> ${actor.lastOffset}`],
                  ['Time', formatProbeTimeRange(actor)],
                  ['Angle', `${formatProbeAngle(actor.firstAngle)} -> ${formatProbeAngle(actor.lastAngle)}`],
                ])}
              </${Box}>
            `)}
          </${Stack}>
        </${Paper}>
      ` : null}
      ${(probe.actorOrder || []).length ? html`
        <${Paper} variant="outlined" sx=${{ p: 1.25, bgcolor: alpha('#fff', 0.56) }}>
          <${Typography} variant="subtitle2" gutterBottom=${true}>Prop Actor Envelopes</${Typography}>
          <${Stack} spacing=${1}>
            ${(probe.actorOrder || []).slice(0, 10).map((actor) => html`
              <${Box} key=${`env-${actor.actorID}-${actor.firstOffset}-${actor.lastOffset}`} sx=${{ pb: 1, borderBottom: '1px solid rgba(23,36,34,0.08)' }}>
                ${detailRows([
                  ['Actor', shortActor(actor.actorID)],
                  ['Samples', actor.sampleCount],
                  ['Offsets', `${actor.firstOffset} -> ${actor.lastOffset}`],
                  ['Gap', actor.gapFromPreviousOffset || '-'],
                ])}
              </${Box}>
            `)}
          </${Stack}>
        </${Paper}>
      ` : null}
    </${Stack}>
  `;
}

function buildRotationGraph(track) {
  if (!track?.samples?.length) {
    return null;
  }
  const points = [];
  let previousRotation = null;
  track.samples.forEach((sample, sampleIndex) => {
    if (!sample.rotation && !sample.rotationDegrees) {
      return;
    }
    const point = {
      sampleIndex,
      offset: sample.offset,
      time: sample.time || '',
      changed: sample.changed || '',
      x: axisValueDegrees(sample, 'x'),
      y: axisValueDegrees(sample, 'y'),
      z: axisValueDegrees(sample, 'z'),
    };
    const changed = sample.changed === 'rotation' || sample.changed === 'state';
    const repeated = previousRotation && rotationPointEquals(previousRotation, point);
    if (!changed && repeated) {
      return;
    }
    points.push({
      ...point,
    });
    previousRotation = point;
  });
  if (points.length === 0) {
    return null;
  }
  const unwrappedPoints = unwrapRotationGraphPoints(points);
  let minValue = Infinity;
  let maxValue = -Infinity;
  unwrappedPoints.forEach((point) => {
    minValue = Math.min(minValue, point.x, point.y, point.z);
    maxValue = Math.max(maxValue, point.x, point.y, point.z);
  });
  if (minValue === maxValue) {
    minValue -= 1;
    maxValue += 1;
  }
  return {
    points: unwrappedPoints,
    minValue,
    maxValue,
    maxSampleIndex: Math.max(track.samples.length - 1, 0),
    actualPointCount: unwrappedPoints.length,
  };
}

function unwrapRotationGraphPoints(points) {
  const unwrap = (axis) => {
    const values = points.map((point) => point[axis]);
    if (values.length === 0) {
      return values;
    }
    const out = [values[0]];
    let offset = 0;
    let previousWrapped = values[0];
    for (let index = 1; index < values.length; index += 1) {
      const value = values[index];
      const delta = value - previousWrapped;
      if (delta > 180) {
        offset -= 360;
      } else if (delta < -180) {
        offset += 360;
      }
      out.push(value + offset);
      previousWrapped = value;
    }
    return out;
  };

  const xs = unwrap('x');
  const ys = unwrap('y');
  const zs = unwrap('z');
  return points.map((point, index) => ({
    ...point,
    x: xs[index],
    y: ys[index],
    z: zs[index],
  }));
}

function rotationGraphPointAtCursor(graph, cursor) {
  const clamped = Math.max(0, cursor);
  let current = graph.points[0];
  for (const point of graph.points) {
    if (point.sampleIndex > clamped) {
      break;
    }
    current = point;
  }
  return current;
}

function rotationPointEquals(left, right) {
  return (
    Math.abs(Number(left?.x || 0) - Number(right?.x || 0)) < 0.0001 &&
    Math.abs(Number(left?.y || 0) - Number(right?.y || 0)) < 0.0001 &&
    Math.abs(Number(left?.z || 0) - Number(right?.z || 0)) < 0.0001
  );
}

function buildDirectionGraph(track, validationCheck, fallbackAxis) {
  if (!track?.samples?.length) {
    return null;
  }
  const points = movementDirectionGraphSamples(track, validationCheck, fallbackAxis);
  if (points.length < 2) {
    return null;
  }
  const unwrappedPoints = unwrapNamedDegreeFields(points, ['movementDegrees', 'viewingDegrees']);
  let minValue = Infinity;
  let maxValue = -Infinity;
  unwrappedPoints.forEach((point) => {
    minValue = Math.min(minValue, point.movementDegrees, point.viewingDegrees);
    maxValue = Math.max(maxValue, point.movementDegrees, point.viewingDegrees);
  });
  if (minValue === maxValue) {
    minValue -= 1;
    maxValue += 1;
  }
  return {
    points: unwrappedPoints.map((point) => ({
      ...point,
      deltaDegrees: wrapDegrees(point.movementDegrees - point.viewingDegrees),
    })),
    minValue,
    maxValue,
    maxSampleIndex: Math.max(track.samples.length - 1, 0),
    summary: buildDirectionSummary(points, validationCheck, fallbackAxis),
  };
}

function movementDirectionGraphSamples(track, validationCheck, fallbackAxis) {
  const positionPoints = [];
  track.samples.forEach((sample, sampleIndex) => {
    if (sample.changed !== 'position' || !sample.position || (!sample.rotation && sample.viewingDirectionDegrees == null)) {
      return;
    }
    positionPoints.push({
      sampleIndex,
      offset: sample.offset ?? sampleIndex,
      position: sample.position,
      rotation: sample.rotation || null,
      viewingDirectionDegrees: sample.viewingDirectionDegrees,
      time: sample.time || '',
    });
  });
  if (positionPoints.length < 2) {
    return [];
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
    const movementDegrees = degreesFromRadians(Math.atan2(dy, dx));
    const viewingDegrees = directionViewingDegrees(current, validationCheck, fallbackAxis);
    if (!Number.isFinite(viewingDegrees)) {
      continue;
    }
    points.push({
      sampleIndex: current.sampleIndex,
      offset: current.offset,
      movementDegrees,
      viewingDegrees,
      time: current.time,
    });
  }
  return points;
}

function buildDirectionSummary(points, validationCheck, fallbackAxis) {
  if (validationCheck) {
    return {
      sourceLabel: formatValidationSource(validationCheck),
      meanErrorDegrees: validationCheck.meanErrorDegrees,
      medianErrorDegrees: validationCheck.medianErrorDegrees,
      p90ErrorDegrees: validationCheck.p90ErrorDegrees,
      meanCosineAgreement: validationCheck.meanCosineAgreement,
      makesSense: Boolean(validationCheck.makesSense),
      stepSamples: validationCheck.stepSamples,
      plottedSamples: points.length,
      fallbackAxis,
    };
  }
  const errors = points.map((point) => Math.abs(wrapDegrees(point.movementDegrees - point.viewingDegrees)));
  const cosineSum = points.reduce((total, point) => total + Math.cos(wrapDegrees(point.movementDegrees - point.viewingDegrees) * Math.PI / 180), 0);
  return {
    sourceLabel: `${AXIS_LABELS[fallbackAxis]} axis fallback`,
    meanErrorDegrees: meanOf(errors),
    medianErrorDegrees: percentileOf(errors, 0.5),
    p90ErrorDegrees: percentileOf(errors, 0.9),
    meanCosineAgreement: cosineSum / Math.max(points.length, 1),
    makesSense: false,
    stepSamples: points.length,
    plottedSamples: points.length,
    fallbackAxis,
  };
}

function directionGraphPointAtCursor(graph, cursor) {
  const clamped = Math.max(0, cursor);
  let current = graph.points[0];
  for (const point of graph.points) {
    if (point.sampleIndex > clamped) {
      break;
    }
    current = point;
  }
  return current;
}

function unwrapNamedDegreeFields(points, fields) {
  const series = Object.fromEntries(fields.map((field) => [field, unwrapDegreeSeries(points.map((point) => point[field]))]));
  return points.map((point, index) => ({
    ...point,
    ...Object.fromEntries(fields.map((field) => [field, series[field][index]])),
  }));
}

function unwrapDegreeSeries(values) {
  if (values.length === 0) {
    return values;
  }
  const out = [values[0]];
  let offset = 0;
  let previousWrapped = values[0];
  for (let index = 1; index < values.length; index += 1) {
    const value = values[index];
    const delta = value - previousWrapped;
    if (delta > 180) {
      offset -= 360;
    } else if (delta < -180) {
      offset += 360;
    }
    out.push(value + offset);
    previousWrapped = value;
  }
  return out;
}

function directionViewingDegrees(point, validationCheck, fallbackAxis) {
  if (validationCheck?.headingSourceGuess === 'derived-heading') {
    if (point?.viewingDirectionDegrees == null) {
      return Number.NaN;
    }
    return wrapDegrees(Number(point.viewingDirectionDegrees || 0) + Number(validationCheck.headingOffsetDegrees || 0));
  }
  return derivedViewingDegrees(point?.rotation, validationCheck, fallbackAxis);
}

function derivedViewingDegrees(rotation, validationCheck, fallbackAxis) {
  if (validationCheck?.headingSourceGuess === 'forward-vector') {
    const forward = vectorForForwardAxis(validationCheck.headingAxisGuess);
    const world = rotateVectorByQuaternion(quaternionFromEulerRadians(rotation), forward);
    return wrapDegrees(degreesFromRadians(Math.atan2(world.y, world.x)) + Number(validationCheck.headingOffsetDegrees || 0));
  }
  if (validationCheck?.headingSourceGuess === 'euler-axis') {
    const axis = validationCheck.headingAxisGuess || fallbackAxis || 'z';
    const sign = Number(validationCheck.headingSignGuess || 1) < 0 ? -1 : 1;
    return wrapDegrees(sign * radiansToWrappedDegrees(rotation?.[axis] || 0) + Number(validationCheck.headingOffsetDegrees || 0));
  }
  const axis = fallbackAxis || 'z';
  return wrapDegrees(radiansToWrappedDegrees(rotation?.[axis] || 0));
}

function quaternionFromEulerRadians(rotation) {
  const roll = Number(rotation?.x || 0);
  const pitch = Number(rotation?.y || 0);
  const yaw = Number(rotation?.z || 0);
  const cy = Math.cos(yaw * 0.5);
  const sy = Math.sin(yaw * 0.5);
  const cp = Math.cos(pitch * 0.5);
  const sp = Math.sin(pitch * 0.5);
  const cr = Math.cos(roll * 0.5);
  const sr = Math.sin(roll * 0.5);
  return {
    w: (cr * cp * cy) + (sr * sp * sy),
    x: (sr * cp * cy) - (cr * sp * sy),
    y: (cr * sp * cy) + (sr * cp * sy),
    z: (cr * cp * sy) - (sr * sp * cy),
  };
}

function rotateVectorByQuaternion(quat, vector) {
  const qx = Number(quat?.x || 0);
  const qy = Number(quat?.y || 0);
  const qz = Number(quat?.z || 0);
  const qw = Number(quat?.w || 1);
  const vx = Number(vector?.x || 0);
  const vy = Number(vector?.y || 0);
  const vz = Number(vector?.z || 0);
  const tx = 2 * ((qy * vz) - (qz * vy));
  const ty = 2 * ((qz * vx) - (qx * vz));
  const tz = 2 * ((qx * vy) - (qy * vx));
  return {
    x: vx + (qw * tx) + (qy * tz) - (qz * ty),
    y: vy + (qw * ty) + (qz * tx) - (qx * tz),
    z: vz + (qw * tz) + (qx * ty) - (qy * tx),
  };
}

function vectorForForwardAxis(name) {
  switch (name) {
    case '+x':
      return { x: 1, y: 0, z: 0 };
    case '-x':
      return { x: -1, y: 0, z: 0 };
    case '+y':
      return { x: 0, y: 1, z: 0 };
    case '-y':
      return { x: 0, y: -1, z: 0 };
    case '+z':
      return { x: 0, y: 0, z: 1 };
    case '-z':
      return { x: 0, y: 0, z: -1 };
    default:
      return { x: 0, y: 1, z: 0 };
  }
}

function axisValueDegrees(sample, axis) {
  if (sample.rotationDegrees && sample.rotationDegrees[axis] != null) {
    return Number(sample.rotationDegrees[axis] || 0);
  }
  return radiansToWrappedDegrees(sample.rotation?.[axis] || 0);
}
function renderSvg(scene, rotationAxis) {
  const viewBox = `0 0 ${SVG_WIDTH} ${SVG_HEIGHT}`;
  const project = createProjector(scene.bounds);
  const polylines = scene.entries.map((entry) => {
    const points = entry.trail.map((item) => project(item.point)).map((point) => `${point.x},${point.y}`).join(' ');
    const currentPoint = entry.currentPoint ? project(entry.currentPoint) : null;
    const angle = entry.currentHeadingDegrees != null
      ? (entry.currentHeadingDegrees * Math.PI / 180)
      : (entry.currentRotation ? headingRadians(entry.currentRotation, rotationAxis) : null);
    const arrow = currentPoint && angle != null
      ? {
          x2: currentPoint.x + Math.cos(angle) * 22,
          y2: currentPoint.y - Math.sin(angle) * 22,
        }
      : null;
    const color = colorForActor(entry.actorID);
    const isFocus = scene.focusTrack?.actorID === entry.actorID;
    return html`
      <g key=${entry.actorID}>
        ${points ? html`<polyline fill="none" stroke=${color} strokeWidth=${isFocus ? '4' : '3'} strokeOpacity=${isFocus ? '0.9' : '0.7'} points=${points}></polyline>` : null}
        ${currentPoint ? html`
          ${isFocus ? html`<circle cx=${currentPoint.x} cy=${currentPoint.y} r="14" fill="none" stroke=${color} strokeWidth="3" strokeOpacity="0.35"></circle>` : null}
          <circle cx=${currentPoint.x} cy=${currentPoint.y} r=${isFocus ? '8' : '6'} fill=${color}></circle>
          <text x=${currentPoint.x + 10} y=${currentPoint.y - 10} fill="#172422" font-size="14" font-weight="700">
            ${displayTrack(entry)}
          </text>
        ` : null}
        ${currentPoint && arrow ? html`
          <line
            x1=${currentPoint.x}
            y1=${currentPoint.y}
            x2=${arrow.x2}
            y2=${arrow.y2}
            stroke=${color}
            strokeWidth="3"
            strokeLinecap="round"
          ></line>
        ` : null}
      </g>
    `;
  });

  return html`
    <svg className="viewer-svg" viewBox=${viewBox} preserveAspectRatio="xMidYMid meet" style=${{ width: '100%', height: '100%', display: 'block' }}>
      <rect width=${SVG_WIDTH} height=${SVG_HEIGHT} fill="transparent"></rect>
      ${gridLines(scene, project)}
      ${polylines}
    </svg>
  `;
}

function gridLines(scene, project) {
  const lines = [];
  const focusPosition = scene.focusPosition || { x: 0, y: 0, z: 0 };
  const horizontalAxis = scene.config.horizontal;
  const verticalAxis = scene.config.vertical;
  const centerHorizontal = Number(focusPosition[horizontalAxis] || 0);
  const centerVertical = Number(focusPosition[verticalAxis] || 0);
  const step = scene.gridStep;
  const minHorizontal = centerHorizontal - scene.bounds.maxHorizontal;
  const maxHorizontal = centerHorizontal + scene.bounds.maxHorizontal;
  const minVertical = centerVertical - scene.bounds.maxVertical;
  const maxVertical = centerVertical + scene.bounds.maxVertical;

  for (
    let value = floorToStep(minHorizontal, step), index = 0;
    value <= maxHorizontal + step * 0.5 && index < 200;
    value += step, index += 1
  ) {
    const relative = value - centerHorizontal;
    const x = project({ horizontal: relative, vertical: 0 }).x;
    const isCenter = Math.abs(relative) < step * 0.001;
    lines.push(html`
      <line
        key=${`vx-${index}`}
        x1=${x}
        x2=${x}
        y1=${PADDING}
        y2=${SVG_HEIGHT - PADDING}
        stroke=${isCenter ? 'rgba(23,36,34,0.28)' : 'rgba(23,36,34,0.1)'}
        strokeWidth=${isCenter ? '1.8' : '1'}
      ></line>
    `);
  }

  for (
    let value = floorToStep(minVertical, step), index = 0;
    value <= maxVertical + step * 0.5 && index < 200;
    value += step, index += 1
  ) {
    const relative = value - centerVertical;
    const y = project({ horizontal: 0, vertical: relative }).y;
    const isCenter = Math.abs(relative) < step * 0.001;
    lines.push(html`
      <line
        key=${`hy-${index}`}
        x1=${PADDING}
        x2=${SVG_WIDTH - PADDING}
        y1=${y}
        y2=${y}
        stroke=${isCenter ? 'rgba(23,36,34,0.28)' : 'rgba(23,36,34,0.1)'}
        strokeWidth=${isCenter ? '1.8' : '1'}
      ></line>
    `);
  }

  lines.push(html`
    <text key="bounds" x="18" y="28" fill="rgba(23,36,34,0.68)" font-size="14">
      ${scene.config.label} plane | focus ${scene.focusTrack ? displayTrack(scene.focusTrack) : 'none'} | ${AXIS_LABELS[horizontalAxis]} ${centerHorizontal.toFixed(2)} | ${AXIS_LABELS[verticalAxis]} ${centerVertical.toFixed(2)}
    </text>
  `);
  lines.push(html`
    <text key="step" x=${SVG_WIDTH - 240} y=${SVG_HEIGHT - 18} fill="rgba(23,36,34,0.55)" font-size="14">
      grid ${step.toFixed(2)} units
    </text>
  `);
  return lines;
}

function buildScene(data, tracks, focusTrack, cursor, trailLength, viewPlane, rotationAxis) {
  const config = planeConfig(viewPlane);
  const focusPosition = positionAt(focusTrack, cursor);
  const entries = tracks.map((track) => {
    const validationCheck = validationCheckForTrack(data, track);
    const trail = trailPoints(track, cursor, trailLength).map((sample) => ({
      sample,
      point: relativePointForPlane(sample.position, focusPosition, config),
    }));
    const currentSample = sampleAt(track, cursor);
    const currentPosition = positionAt(track, cursor);
    const currentRotation = rotationAt(track, cursor);
    const currentHeading = headingStateAt(track, cursor, validationCheck, rotationAxis);
    return {
      track,
      actorID: track.actorID,
      trail,
      currentSample,
      currentPosition,
      currentRotation,
      currentHeadingDegrees: currentHeading?.degrees ?? null,
      currentHeadingSource: currentHeading?.source || null,
      currentPoint: currentPosition ? relativePointForPlane(currentPosition, focusPosition, config) : null,
    };
  });
  const bounds = computeBounds(entries);
  const focusEntry = entries.find((entry) => entry.actorID === focusTrack?.actorID) || null;
  return {
    config,
    focusTrack,
    focusPosition,
    entries,
    bounds,
    focusHeadingSource: focusEntry?.currentHeadingSource || null,
    gridStep: niceGridStep(Math.max(bounds.maxHorizontal, bounds.maxVertical)),
  };
}

function summarizeTrack(entry, focusPosition, config, rotationAxis) {
  if (!entry.currentSample && !entry.currentPosition && !entry.currentRotation && entry.currentHeadingDegrees == null) {
    return null;
  }
  return {
    actorID: entry.actorID,
    changed: entry.currentSample?.changed || 'state',
    offset: entry.currentSample?.offset ?? '-',
    position: entry.currentPosition ? formatVector(entry.currentPosition) : '-',
    relative: entry.currentPosition ? formatPlaneOffset(entry.currentPosition, focusPosition, config) : '-',
    distance: entry.currentPoint ? `${Math.hypot(entry.currentPoint.horizontal, entry.currentPoint.vertical).toFixed(3)} units` : '-',
    rotation: entry.currentRotation ? formatVector(entry.currentRotation) : '-',
    heading: entry.currentHeadingDegrees != null
      ? `${entry.currentHeadingDegrees.toFixed(1)}deg`
      : (entry.currentRotation ? `${degreesFromRadians(headingRadians(entry.currentRotation, rotationAxis)).toFixed(1)}deg (${AXIS_LABELS[rotationAxis]})` : '-'),
    headingSource: entry.currentHeadingSource || (entry.currentRotation ? `Raw ${AXIS_LABELS[rotationAxis]} axis` : '-'),
    time: entry.currentSample?.time || 'Untimed',
  };
}

function trailPoints(track, cursor, trailLength) {
  if (!track?.samples?.length) {
    return [];
  }
  const capped = track.samples.slice(0, Math.min(cursor + 1, track.samples.length));
  return capped.filter((sample) => sample.position).slice(-trailLength);
}

function sampleAt(track, cursor) {
  if (!track?.samples?.length) {
    return null;
  }
  return track.samples[Math.min(cursor, track.samples.length - 1)];
}

function positionAt(track, cursor) {
  return latestField(track, cursor, 'position');
}

function rotationAt(track, cursor) {
  return latestField(track, cursor, 'rotation');
}

function headingStateAt(track, cursor, validationCheck, fallbackAxis) {
  if (!track?.samples?.length) {
    return null;
  }
  for (let index = Math.min(cursor, track.samples.length - 1); index >= 0; index -= 1) {
    const sample = track.samples[index];
    const degrees = directionViewingDegrees(sample, validationCheck, fallbackAxis);
    if (!Number.isFinite(degrees)) {
      continue;
    }
    return {
      degrees,
      source: headingSourceLabel(sample, validationCheck, fallbackAxis),
    };
  }
  return null;
}

function headingSourceLabel(sample, validationCheck, fallbackAxis) {
  if (validationCheck?.headingSourceGuess === 'derived-heading' && sample?.viewingDirectionDegrees != null) {
    return 'Recovered view heading';
  }
  if (validationCheck?.headingSourceGuess === 'forward-vector') {
    return 'Forward-vector heading';
  }
  if (validationCheck?.headingSourceGuess === 'euler-axis') {
    return `Raw ${AXIS_LABELS[validationCheck.headingAxisGuess || fallbackAxis || 'z']} axis`;
  }
  return `Raw ${AXIS_LABELS[fallbackAxis || 'z']} axis`;
}

function latestField(track, cursor, field) {
  if (!track?.samples?.length) {
    return null;
  }
  for (let index = Math.min(cursor, track.samples.length - 1); index >= 0; index -= 1) {
    if (track.samples[index]?.[field]) {
      return track.samples[index][field];
    }
  }
  return null;
}

function relativePointForPlane(position, focusPosition, config) {
  const origin = focusPosition || { x: 0, y: 0, z: 0 };
  return {
    horizontal: Number(position?.[config.horizontal] || 0) - Number(origin?.[config.horizontal] || 0),
    vertical: Number(position?.[config.vertical] || 0) - Number(origin?.[config.vertical] || 0),
  };
}

function computeBounds(entries) {
  let maxHorizontal = 1;
  let maxVertical = 1;
  let hasPoints = false;

  entries.forEach((entry) => {
    entry.trail.forEach(({ point }) => {
      hasPoints = true;
      maxHorizontal = Math.max(maxHorizontal, Math.abs(point.horizontal));
      maxVertical = Math.max(maxVertical, Math.abs(point.vertical));
    });
    if (entry.currentPoint) {
      hasPoints = true;
      maxHorizontal = Math.max(maxHorizontal, Math.abs(entry.currentPoint.horizontal));
      maxVertical = Math.max(maxVertical, Math.abs(entry.currentPoint.vertical));
    }
  });

  if (!hasPoints) {
    return { maxHorizontal: 10, maxVertical: 10 };
  }

  return {
    maxHorizontal: Math.max(1, maxHorizontal * 1.12),
    maxVertical: Math.max(1, maxVertical * 1.12),
  };
}

function createProjector(bounds) {
  const halfWidth = (SVG_WIDTH - PADDING * 2) / 2;
  const halfHeight = (SVG_HEIGHT - PADDING * 2) / 2;
  const scale = Math.min(
    halfWidth / Math.max(bounds.maxHorizontal, 0.001),
    halfHeight / Math.max(bounds.maxVertical, 0.001),
  );
  const centerX = SVG_WIDTH / 2;
  const centerY = SVG_HEIGHT / 2;
  return (point) => ({
    x: centerX + point.horizontal * scale,
    y: centerY - point.vertical * scale,
  });
}

function planeConfig(viewPlane) {
  return VIEW_PLANES[viewPlane] || VIEW_PLANES.xy;
}

function describePlane(config) {
  return `${config.label} (${AXIS_LABELS[config.horizontal]} / ${AXIS_LABELS[config.vertical]})`;
}

function niceGridStep(radius) {
  const rough = Math.max(radius / 6, 0.001);
  const power = 10 ** Math.floor(Math.log10(rough));
  const normalized = rough / power;
  if (normalized <= 1) {
    return power;
  }
  if (normalized <= 2) {
    return 2 * power;
  }
  if (normalized <= 5) {
    return 5 * power;
  }
  return 10 * power;
}

function floorToStep(value, step) {
  return Math.floor(value / step) * step;
}

function headingRadians(rotation, axis) {
  const value = Number(rotation?.[axis] || 0);
  return normalizeRadians(value);
}

function normalizeRadians(value) {
  const fullTurn = Math.PI * 2;
  let wrapped = Number(value || 0) % fullTurn;
  if (wrapped <= -Math.PI) {
    wrapped += fullTurn;
  }
  if (wrapped > Math.PI) {
    wrapped -= fullTurn;
  }
  return wrapped;
}

function radiansToWrappedDegrees(value) {
  return degreesFromRadians(normalizeRadians(value));
}

function wrapDegrees(value) {
  let wrapped = Number(value || 0) % 360;
  if (wrapped <= -180) {
    wrapped += 360;
  }
  if (wrapped > 180) {
    wrapped -= 360;
  }
  return wrapped;
}

function formatVector(vector) {
  return `${vector.x.toFixed(3)}, ${vector.y.toFixed(3)}, ${vector.z.toFixed(3)}`;
}

function formatPlaneOffset(position, focusPosition, config) {
  const origin = focusPosition || { x: 0, y: 0, z: 0 };
  const horizontal = Number(position?.[config.horizontal] || 0) - Number(origin?.[config.horizontal] || 0);
  const vertical = Number(position?.[config.vertical] || 0) - Number(origin?.[config.vertical] || 0);
  return `${AXIS_LABELS[config.horizontal]} ${horizontal.toFixed(3)} / ${AXIS_LABELS[config.vertical]} ${vertical.toFixed(3)}`;
}

function colorForActor(actorID) {
  const hash = Array.from(actorID).reduce((total, char) => (total * 31 + char.charCodeAt(0)) % 360, 17);
  return `hsl(${hash} 72% 46%)`;
}

function displayTrack(track) {
  if (!track) {
    return '-';
  }
  if (track.playerNameGuess) {
    return track.label ? `${track.playerNameGuess} (${track.label})` : track.playerNameGuess;
  }
  if (track.label) {
    return track.label;
  }
  return shortActor(track.actorID);
}

function shortProp(propID) {
  if (!propID) {
    return '-';
  }
  return propID.length <= 16 ? propID : `${propID.slice(0, 8)}...${propID.slice(-8)}`;
}

function shortActor(actorID) {
  if (!actorID) {
    return '-';
  }
  return actorID.length <= 10 ? actorID : `${actorID.slice(0, 8)}...${actorID.slice(-4)}`;
}

function basename(filePath) {
  const parts = filePath.split(/[\\/]/);
  return parts[parts.length - 1] || filePath;
}

function totalSamples(track) {
  return track?.samples?.length || 0;
}

function maxTrackLength(tracks) {
  return tracks.reduce((max, track) => Math.max(max, totalSamples(track)), 0);
}

function degreesFromRadians(value) {
  return (value * 180) / Math.PI;
}

function validationCheckForTrack(data, track) {
  if (!track || !Array.isArray(data?.validation?.trackChecks)) {
    return null;
  }
  return data.validation.trackChecks.find((check) => check.actorID === track.actorID) || null;
}

function directionSearchForTrack(data, track) {
  if (!track || !data?.directionSearch) {
    return null;
  }
  return data.directionSearch.trackActorID === track.actorID ? data.directionSearch : null;
}

function formatValidationSource(validationCheck) {
  if (!validationCheck) {
    return '-';
  }
  if (validationCheck.headingSourceGuess === 'forward-vector') {
    return `${validationCheck.headingSourceGuess} ${validationCheck.headingAxisGuess || ''}`.trim();
  }
  if (validationCheck.headingSourceGuess === 'euler-axis') {
    const sign = Number(validationCheck.headingSignGuess || 1) < 0 ? '-' : '+';
    return `${validationCheck.headingSourceGuess} ${sign}${validationCheck.headingAxisGuess || 'z'}`;
  }
  return validationCheck.headingSourceGuess || '-';
}

function directionCandidateRows(candidate) {
  return [
    ['Source', `${candidate.source || '-'} / ${candidate.plane || '-'}`],
    ['Prop', shortProp(candidate.propID)],
    ['Actor', shortActor(candidate.actorID)],
    ['Vector offset', candidate.vectorOffset],
    ['Samples', candidate.sampleCount],
    ['Offset', formatSignedDegrees(candidate.headingOffsetDegrees)],
    ['Mean error', formatDegrees(candidate.meanErrorDegrees)],
    ['Median error', formatDegrees(candidate.medianErrorDegrees)],
    ['P90 error', formatDegrees(candidate.p90ErrorDegrees)],
    ['Mean cosine', formatMetric(candidate.meanCosineAgreement)],
    ['Makes sense', candidate.makesSense ? 'yes' : 'no'],
  ];
}

function probeLaneKey(group) {
  return `${group?.source || ''}|${group?.vectorOffset ?? ''}|${group?.plane || ''}`;
}

function formatProbeTimeRange(actor) {
  if (actor?.firstMilliseconds == null || actor?.lastMilliseconds == null) {
    return '-';
  }
  return `${formatMillisecondsLabel(actor.firstMilliseconds)} -> ${formatMillisecondsLabel(actor.lastMilliseconds)}`;
}

function formatProbeAngle(value) {
  return Number.isFinite(value) ? formatSignedDegrees(value) : '-';
}

function formatDegrees(value) {
  return `${Number(value || 0).toFixed(1)}deg`;
}

function formatSignedDegrees(value) {
  const numeric = Number(value || 0);
  return `${numeric >= 0 ? '+' : ''}${numeric.toFixed(1)}deg`;
}

function formatMetric(value) {
  return Number(value || 0).toFixed(3);
}

function formatPercent(value) {
  return `${Number(value || 0).toFixed(1)}%`;
}

function formatDurationShort(value) {
  const totalSeconds = Math.max(0, Math.round(Number(value || 0) / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  if (minutes > 0) {
    return `${minutes}m ${seconds}s`;
  }
  return `${seconds}s`;
}

function sleep(milliseconds) {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds));
}

function formatUsageHeadline(usage) {
  if (!usage) {
    return '-';
  }
  return `${formatPercent(usage.selectedPropUsagePercent)} sel`;
}

function formatUsageDetail(usage) {
  if (!usage) {
    return 'No replay-usage accounting attached yet';
  }
  return `${formatPercent(usage.replayUsagePercent)} of ${usage.totalCandidatePackets} candidate packets, ${formatPercent(usage.primarySharePercent)} kept in primary tracks`;
}

function sampleTimeAtCursor(track, cursor) {
  if (!track?.samples?.length) {
    return '';
  }
  for (let index = Math.min(cursor, track.samples.length - 1); index >= 0; index -= 1) {
    if (track.samples[index]?.time) {
      return track.samples[index].time;
    }
  }
  return '';
}

function formatClockHeadline(clock) {
  if (!clock) {
    return '-';
  }
  return `${Number(clock.estimatedTickRate || 0).toFixed(2)} Hz`;
}

function formatClockDetail(clock, currentTime) {
  if (!clock) {
    return 'No replay-clock summary attached yet';
  }
  const current = currentTime ? `cursor ${currentTime} / ` : '';
  return `${current}${formatClockRange(clock)} / ${clock.mapping || 'mapping n/a'}`;
}

function formatClockRange(clock) {
  if (!clock) {
    return '-';
  }
  return `${formatMillisecondsLabel(clock.firstMillisecondsRemaining)} -> ${formatMillisecondsLabel(clock.lastMillisecondsRemaining)}`;
}

function formatMillisecondsLabel(value) {
  const milliseconds = Number(value);
  if (!Number.isFinite(milliseconds) || milliseconds < 0) {
    return '-';
  }
  const totalSeconds = Math.floor(milliseconds / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  const millis = Math.floor(milliseconds % 1000);
  return `${minutes}:${String(seconds).padStart(2, '0')}.${String(millis).padStart(3, '0')}`;
}

function agreementChipColor(summary) {
  if (!summary) {
    return 'default';
  }
  if (summary.makesSense || Number(summary.meanCosineAgreement || 0) >= 0.6) {
    return 'success';
  }
  if (Number(summary.meanCosineAgreement || 0) >= 0.3) {
    return 'warning';
  }
  return 'error';
}

function meanOf(values) {
  if (!values.length) {
    return 0;
  }
  return values.reduce((total, value) => total + Number(value || 0), 0) / values.length;
}

function percentileOf(values, percentile) {
  if (!values.length) {
    return 0;
  }
  const sorted = [...values].sort((left, right) => left - right);
  const index = Math.floor((sorted.length - 1) * percentile);
  return sorted[Math.max(0, Math.min(index, sorted.length - 1))];
}

function renderConversionStatus(state, onLoadOutput) {
  if (!state) {
    return null;
  }
  if (state.status === 'running') {
    const expectedCount = Number(state.expectedCount || (state.generatedFiles || []).length || 0);
    const generatedCount = Number(state.generatedCount || 0);
    const determinate = expectedCount > 1;
    return html`
      <${Alert} severity="info">
        <${Stack} spacing=${1.1}>
          <${Typography} variant="body2">
            ${describeConversionKind(state.kind)} for ${basename(state.inputPath || '')} is running locally.
          </${Typography}>
          <${LinearProgress}
            variant=${determinate ? 'determinate' : 'indeterminate'}
            value=${determinate ? Number(state.progressPercent || 0) : undefined}
          />
          ${detailRows([
            ['Elapsed', formatDurationShort(state.elapsedMilliseconds)],
            ['Files', `${generatedCount}/${expectedCount || 1}`],
            ['Progress', determinate ? formatPercent(state.progressPercent) : 'processing'],
          ])}
          <${Typography} variant="caption" color="text.secondary">
            Job ${state.jobId || '-'}${state.generatedFiles?.length ? ` / ${Math.max(expectedCount - generatedCount, 0)} file(s) still pending` : ''}
          </${Typography}>
        </${Stack}>
      </${Alert}>
    `;
  }
  if (state.status === 'error') {
    return html`
      <${Alert} severity="error">
        ${describeConversionKind(state.kind)} failed for ${basename(state.inputPath || '')}: ${state.message || 'Unknown error'}
      </${Alert}>
    `;
  }
  const generatedFiles = (state.generatedFiles || []).filter((file) => file.exists !== false);
  const visibleFiles = generatedFiles.slice(0, 6);
  const hiddenCount = Math.max(0, generatedFiles.length - visibleFiles.length);
  return html`
    <${Alert} severity="success">
      <${Stack} spacing=${1}>
        <${Typography} variant="body2">
          ${describeConversionKind(state.kind)} finished for ${basename(state.inputPath || '')}.
        </${Typography}>
        ${generatedFiles.length ? html`
          <${Stack} direction="row" spacing=${1} flexWrap="wrap" useFlexGap=${true}>
            ${visibleFiles.map((file) => html`
              <${Button}
                key=${file.path + '-open'}
                size="small"
                variant="outlined"
                component="a"
                href=${file.webPath}
                target="_blank"
                rel="noreferrer"
              >
                Open ${describeGeneratedKind(file.kind)}
              </${Button}>
            `)}
            ${visibleFiles.filter((file) => file.kind === 'transform-json').map((file) => html`
              <${Button}
                key=${file.path + '-load'}
                size="small"
                variant="contained"
                onClick=${() => onLoadOutput(file.webPath)}
              >
                Load ${basename(file.path)}
              </${Button}>
            `)}
          </${Stack}>
        ` : null}
        ${hiddenCount > 0 ? html`
          <${Typography} variant="caption" color="text.secondary">
            ${hiddenCount} more files were generated in the same batch.
          </${Typography}>
        ` : null}
        ${state.stderr ? html`
          <${Typography} variant="caption" color="text.secondary">
            ${state.stderr}
          </${Typography}>
        ` : null}
      </${Stack}>
    </${Alert}>
  `;
}

function describeConversionKind(kind) {
  if (kind === 'movement') {
    return 'Movement export';
  }
  if (kind === 'movement-probe') {
    return 'Probe export';
  }
  if (kind === 'blender-camera') {
    return 'Blender camera export';
  }
  return kind || 'Conversion';
}

function describeGeneratedKind(kind) {
  if (kind === 'transform-json') {
    return 'transform';
  }
  if (kind === 'probe-json') {
    return 'probe';
  }
  if (kind === 'camera-script') {
    return 'camera';
  }
  return 'file';
}

createRoot(document.getElementById('root')).render(html`<${App} />`);

























