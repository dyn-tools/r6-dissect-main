const http = require('http');
const fs = require('fs');
const path = require('path');
const { spawn } = require('child_process');

const rootDir = path.resolve(__dirname, '..');
const viewerDir = path.resolve(rootDir, 'viewer');
const port = Number(process.env.PORT || 4173);
const goCacheDir = path.join(rootDir, '.gocache');
const goTmpDir = path.join(rootDir, '.gotmp');
const conversionJobs = new Map();
let nextJobId = 1;

const contentTypes = {
  '.css': 'text/css; charset=utf-8',
  '.html': 'text/html; charset=utf-8',
  '.ico': 'image/x-icon',
  '.js': 'text/javascript; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.map': 'application/json; charset=utf-8',
  '.png': 'image/png',
  '.py': 'text/plain; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.txt': 'text/plain; charset=utf-8',
};

function sendJson(res, status, payload) {
  const body = JSON.stringify(payload, null, 2);
  res.writeHead(status, {
    'Content-Length': Buffer.byteLength(body),
    'Content-Type': 'application/json; charset=utf-8',
    'Cache-Control': 'no-store',
  });
  res.end(body);
}

function sendFile(res, filePath) {
  const ext = path.extname(filePath).toLowerCase();
  const stream = fs.createReadStream(filePath);
  stream.on('error', () => sendJson(res, 500, { error: 'Failed to read file.' }));
  res.writeHead(200, {
    'Content-Type': contentTypes[ext] || 'application/octet-stream',
    'Cache-Control': ext === '.json' ? 'no-store' : 'no-cache',
  });
  stream.pipe(res);
}

function normalizeRelativePath(value) {
  return String(value || '').replace(/\\/g, '/').replace(/^\/+/, '').trim();
}

function safeResolve(relativePath) {
  const normalized = normalizeRelativePath(relativePath);
  const target = path.resolve(rootDir, normalized || '.');
  if (target !== rootDir && !target.startsWith(rootDir + path.sep)) {
    throw new Error('Path traversal blocked');
  }
  return target;
}

function relativeFromAbsolute(absolutePath) {
  const relative = path.relative(rootDir, absolutePath);
  if (relative.startsWith('..') || path.isAbsolute(relative)) {
    throw new Error('Path traversal blocked');
  }
  return normalizeRelativePath(relative);
}

function toWebPath(relativePath) {
  return '/' + normalizeRelativePath(relativePath);
}

function sortByPath(entries) {
  return entries.sort((left, right) => left.path.localeCompare(right.path));
}

function createLibraryEntry(relativePath, extra) {
  const normalized = normalizeRelativePath(relativePath);
  return {
    name: path.basename(normalized),
    path: normalized,
    webPath: toWebPath(normalized),
    ...extra,
  };
}

function walkLibrary(dirPath, basePrefix, buckets) {
  const entries = fs.readdirSync(dirPath, { withFileTypes: true });
  let replayCount = 0;
  for (const entry of entries) {
    const absolute = path.join(dirPath, entry.name);
    const relative = normalizeRelativePath(path.posix.join(basePrefix, entry.name));
    if (entry.isDirectory()) {
      const nestedReplayCount = walkLibrary(absolute, relative, buckets);
      if (nestedReplayCount > 0) {
        buckets.replayFolders.push(createLibraryEntry(relative, { replayCount: nestedReplayCount }));
        replayCount += nestedReplayCount;
      }
      continue;
    }
    if (!entry.isFile()) {
      continue;
    }
    const lower = entry.name.toLowerCase();
    if (lower.endsWith('.rec')) {
      replayCount += 1;
      buckets.replayFiles.push(createLibraryEntry(relative, { kind: 'replay-file' }));
      continue;
    }
    if (lower.endsWith('.transforms.json')) {
      buckets.transformFiles.push(createLibraryEntry(relative, { kind: 'transform-json' }));
      continue;
    }
    if (lower.endsWith('.probe.json')) {
      buckets.probeFiles.push(createLibraryEntry(relative, { kind: 'probe-json' }));
      continue;
    }
    if (lower.endsWith('.camera.py')) {
      buckets.cameraFiles.push(createLibraryEntry(relative, { kind: 'camera-script' }));
      continue;
    }
    if (lower.endsWith('.json')) {
      buckets.otherJsonFiles.push(createLibraryEntry(relative, { kind: 'json' }));
    }
  }
  return replayCount;
}

function buildLibrary(rootRelativePath) {
  const requestedDir = normalizeRelativePath(rootRelativePath) || 'replays';
  const absoluteDir = safeResolve(requestedDir);
  const stats = fs.statSync(absoluteDir);
  if (!stats.isDirectory()) {
    throw new Error('Requested path is not a directory.');
  }
  const buckets = {
    replayFiles: [],
    replayFolders: [],
    transformFiles: [],
    probeFiles: [],
    cameraFiles: [],
    otherJsonFiles: [],
  };
  const rootReplayCount = walkLibrary(absoluteDir, requestedDir, buckets);
  if (rootReplayCount > 0) {
    buckets.replayFolders.unshift(createLibraryEntry(requestedDir, { replayCount: rootReplayCount }));
  }
  return {
    root: requestedDir,
    replayFiles: sortByPath(buckets.replayFiles),
    replayFolders: sortByPath(buckets.replayFolders),
    transformFiles: sortByPath(buckets.transformFiles),
    probeFiles: sortByPath(buckets.probeFiles),
    cameraFiles: sortByPath(buckets.cameraFiles),
    otherJsonFiles: sortByPath(buckets.otherJsonFiles),
  };
}

function walkJson(dirPath, basePrefix) {
  const library = {
    replayFiles: [],
    replayFolders: [],
    transformFiles: [],
    probeFiles: [],
    cameraFiles: [],
    otherJsonFiles: [],
  };
  walkLibrary(dirPath, basePrefix, library);
  return sortByPath([
    ...library.transformFiles,
    ...library.probeFiles,
    ...library.otherJsonFiles,
  ]).map(({ name, webPath }) => ({ name, path: webPath }));
}

function ensureRuntimeDirs() {
  fs.mkdirSync(goCacheDir, { recursive: true });
  fs.mkdirSync(goTmpDir, { recursive: true });
}

function trimOutput(text) {
  return String(text || '').trim();
}

function readJsonBody(req) {
  return new Promise((resolve, reject) => {
    let body = '';
    req.on('data', (chunk) => {
      body += chunk;
      if (body.length > 1024 * 1024) {
        reject(new Error('Request body too large.'));
        req.destroy();
      }
    });
    req.on('end', () => {
      if (!body.trim()) {
        resolve({});
        return;
      }
      try {
        resolve(JSON.parse(body));
      } catch (error) {
        reject(new Error('Expected valid JSON request body.'));
      }
    });
    req.on('error', reject);
  });
}

function movementOutputName(relativeInputPath) {
  const extension = path.extname(relativeInputPath);
  if (!extension) {
    return relativeInputPath + '.transforms.json';
  }
  return relativeInputPath.slice(0, -extension.length) + '.transforms.json';
}

function probeOutputName(relativeInputPath) {
  const extension = path.extname(relativeInputPath);
  if (!extension) {
    return relativeInputPath + '.probe.json';
  }
  return relativeInputPath.slice(0, -extension.length) + '.probe.json';
}

function cameraOutputName(relativeInputPath) {
  const extension = path.extname(relativeInputPath);
  if (!extension) {
    return relativeInputPath + '.camera.py';
  }
  return relativeInputPath.slice(0, -extension.length) + '.camera.py';
}

function expectedOutputsForRequest(inputPath, kind, stat, explicitOutputPath) {
  const requestedOutput = normalizeRelativePath(explicitOutputPath);
  if (kind === 'movement' && stat.isDirectory()) {
    const files = buildLibrary(inputPath).replayFiles.map((entry) => createLibraryEntry(
      requestedOutput ? path.posix.join(requestedOutput, path.basename(movementOutputName(entry.path))) : movementOutputName(entry.path),
      { kind: 'transform-json' },
    ));
    return files;
  }
  if (kind === 'movement') {
    return [createLibraryEntry(requestedOutput || movementOutputName(inputPath), { kind: 'transform-json' })];
  }
  if (kind === 'movement-probe') {
    return [createLibraryEntry(requestedOutput || probeOutputName(inputPath), { kind: 'probe-json' })];
  }
  if (kind === 'blender-camera') {
    return [createLibraryEntry(requestedOutput || cameraOutputName(inputPath), { kind: 'camera-script' })];
  }
  throw new Error('Unknown conversion kind.');
}

function goArgsForRequest(kind, inputPath, outputPath) {
  const args = ['run', '.'];
  if (kind === 'movement') {
    args.push('--movement');
  } else if (kind === 'movement-probe') {
    args.push('--movement-probe');
  } else if (kind === 'blender-camera') {
    args.push('--blender-camera');
  } else {
    throw new Error('Unknown conversion kind.');
  }
  const normalizedOutput = normalizeRelativePath(outputPath);
  if (normalizedOutput) {
    args.push('-o', normalizedOutput);
  }
  args.push(normalizeRelativePath(inputPath));
  return args;
}

function buildGeneratedFiles(outputs) {
  return outputs.map((entry) => {
    let exists = false;
    try {
      exists = fs.statSync(safeResolve(entry.path)).isFile();
    } catch (error) {
      exists = false;
    }
    return { ...entry, exists };
  });
}

function createJobId() {
  const sequence = (nextJobId++).toString(36);
  return `job-${Date.now().toString(36)}-${sequence}`;
}

function snapshotJob(job) {
  const generatedFiles = buildGeneratedFiles(job.outputs);
  const generatedCount = generatedFiles.filter((file) => file.exists).length;
  const expectedCount = generatedFiles.length;
  return {
    ok: true,
    jobId: job.id,
    status: job.status,
    kind: job.kind,
    inputPath: job.inputPath,
    outputPath: job.outputPath || null,
    generatedFiles,
    generatedCount,
    expectedCount,
    progressPercent: expectedCount > 0 ? (generatedCount / expectedCount) * 100 : 0,
    primaryOutput: generatedFiles[0] || null,
    stdout: trimOutput(job.stdout),
    stderr: trimOutput(job.stderr),
    message: job.message || '',
    args: job.args,
    startedAt: job.startedAt,
    completedAt: job.completedAt || null,
    elapsedMilliseconds: (job.completedAt || Date.now()) - job.startedAt,
  };
}

function startGoJob(kind, inputPath, outputPath, outputs, args) {
  ensureRuntimeDirs();
  const job = {
    id: createJobId(),
    status: 'running',
    kind,
    inputPath,
    outputPath,
    outputs,
    args,
    stdout: '',
    stderr: '',
    message: '',
    startedAt: Date.now(),
    completedAt: 0,
  };
  conversionJobs.set(job.id, job);
  const child = spawn('go', args, {
    cwd: rootDir,
    env: {
      ...process.env,
      GOCACHE: goCacheDir,
      TEMP: goTmpDir,
      TMP: goTmpDir,
    },
  });
  job.child = child;
  child.stdout.on('data', (chunk) => {
    job.stdout += chunk.toString();
  });
  child.stderr.on('data', (chunk) => {
    job.stderr += chunk.toString();
  });
  child.on('error', (error) => {
    job.status = 'error';
    job.message = error.message;
    job.completedAt = Date.now();
  });
  child.on('close', (code) => {
    job.completedAt = Date.now();
    if (code === 0) {
      job.status = 'success';
      return;
    }
    job.status = 'error';
    job.message = trimOutput(job.stderr) || trimOutput(job.stdout) || `go exited with code ${code}`;
  });
  return snapshotJob(job);
}

function runGoCommand(args) {
  ensureRuntimeDirs();
  return new Promise((resolve, reject) => {
    const child = spawn('go', args, {
      cwd: rootDir,
      env: {
        ...process.env,
        GOCACHE: goCacheDir,
        TEMP: goTmpDir,
        TMP: goTmpDir,
      },
    });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', (chunk) => {
      stdout += chunk.toString();
    });
    child.stderr.on('data', (chunk) => {
      stderr += chunk.toString();
    });
    child.on('error', (error) => {
      reject(error);
    });
    child.on('close', (code) => {
      if (code === 0) {
        resolve({ stdout: trimOutput(stdout), stderr: trimOutput(stderr) });
        return;
      }
      const message = trimOutput(stderr) || trimOutput(stdout) || `go exited with code ${code}`;
      reject(new Error(message));
    });
  });
}

async function handleConvert(req, res) {
  try {
    const body = await readJsonBody(req);
    const kind = body.kind;
    const inputPath = normalizeRelativePath(body.inputPath);
    const outputPath = normalizeRelativePath(body.outputPath);
    if (!inputPath) {
      sendJson(res, 400, { error: 'inputPath is required.' });
      return;
    }
    if (!['movement', 'movement-probe', 'blender-camera'].includes(kind)) {
      sendJson(res, 400, { error: 'Unsupported conversion kind.' });
      return;
    }
    const absoluteInputPath = safeResolve(inputPath);
    const stat = fs.statSync(absoluteInputPath);
    if (stat.isDirectory() && kind !== 'movement') {
      sendJson(res, 400, { error: `${kind} requires a replay file, not a directory.` });
      return;
    }
    if (outputPath) {
      safeResolve(outputPath);
    }
    const args = goArgsForRequest(kind, inputPath, outputPath);
    const outputs = expectedOutputsForRequest(inputPath, kind, stat, outputPath);
    const snapshot = startGoJob(kind, inputPath, outputPath, outputs, args);
    sendJson(res, 202, snapshot);
  } catch (error) {
    sendJson(res, 500, { error: error.message });
  }
}

function handleConvertStatus(req, res, url) {
  const id = String(url.searchParams.get('id') || '').trim();
  if (!id) {
    sendJson(res, 400, { error: 'id is required.' });
    return;
  }
  const job = conversionJobs.get(id);
  if (!job) {
    sendJson(res, 404, { error: 'Conversion job not found.' });
    return;
  }
  sendJson(res, 200, snapshotJob(job));
}

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, `http://${req.headers.host || '127.0.0.1'}`);

  if (url.pathname === '/') {
    res.writeHead(302, { Location: '/viewer/' });
    res.end();
    return;
  }

  if (url.pathname === '/__viewer/library') {
    const requestedDir = url.searchParams.get('dir') || 'replays';
    try {
      sendJson(res, 200, buildLibrary(requestedDir));
      return;
    } catch (error) {
      sendJson(res, 404, { error: error.message });
      return;
    }
  }

  if (url.pathname === '/__viewer/convert') {
    if (req.method !== 'POST') {
      sendJson(res, 405, { error: 'Use POST for conversion requests.' });
      return;
    }
    await handleConvert(req, res);
    return;
  }

  if (url.pathname === '/__viewer/convert-status') {
    if (req.method !== 'GET') {
      sendJson(res, 405, { error: 'Use GET for status requests.' });
      return;
    }
    handleConvertStatus(req, res, url);
    return;
  }

  if (url.pathname === '/__viewer/list-json') {
    const requestedDir = url.searchParams.get('dir') || 'replays';
    try {
      const absoluteDir = safeResolve(requestedDir);
      const stats = fs.statSync(absoluteDir);
      if (!stats.isDirectory()) {
        sendJson(res, 400, { error: 'Requested path is not a directory.' });
        return;
      }
      const files = walkJson(absoluteDir, requestedDir);
      sendJson(res, 200, { files });
      return;
    } catch (error) {
      sendJson(res, 404, { error: error.message });
      return;
    }
  }

  let relativePath = url.pathname;
  if (relativePath === '/viewer') {
    res.writeHead(302, { Location: '/viewer/' });
    res.end();
    return;
  }
  if (relativePath.endsWith('/')) {
    relativePath += 'index.html';
  }

  try {
    let absolutePath = safeResolve(relativePath);
    if (absolutePath === viewerDir) {
      absolutePath = path.join(viewerDir, 'index.html');
    }
    const stats = fs.statSync(absolutePath);
    if (stats.isDirectory()) {
      sendFile(res, path.join(absolutePath, 'index.html'));
      return;
    }
    sendFile(res, absolutePath);
  } catch (error) {
    sendJson(res, 404, { error: 'Not found.' });
  }
});

server.listen(port, '127.0.0.1', () => {
  console.log(`Viewer available at http://127.0.0.1:${port}/viewer/`);
});
