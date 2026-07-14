#!/usr/bin/env node
/**
 * LemonHarness Delegate Runner
 *
 * Simplified sub-agent runner using pi CLI's print mode.
 * Spawns `pi -p` as a subprocess to execute a bounded task.
 *
 * Usage:
 *   echo '{"task":"...","cwd":"...","budgetMs":120000}' | node .lemonharness/delegate-runner.mjs
 *
 * Input (JSON from stdin):
 *   task         — What to accomplish
 *   cwd          — Working directory
 *   budgetMs     — Max execution time in ms (default: 120000)
 *   context      — Additional context for the sub-agent
 *   constraint   — Scope constraints
 *   outputDir    — Where to write result files
 *
 * Output (JSON lines on stdout, final result on last line):
 *   {"type":"log","text":"..."}
 *   {"type":"result","success":true/false,"summary":"...","files":[],"toolCalls":0}
 */

import { writeFile, mkdir } from 'node:fs/promises';
import { spawn } from 'node:child_process';
import { join, resolve } from 'node:path';

// ── Helpers ───────────────────────────────────────────────────────────

function log(...args) {
  const msg = args.map(a => typeof a === 'object' ? JSON.stringify(a) : String(a)).join(' ');
  process.stdout.write(JSON.stringify({ type: 'log', text: msg }) + '\n');
}

function emitResult(result) {
  process.stdout.write(JSON.stringify({ type: 'result', ...result }) + '\n');
}

async function readStdin() {
  const chunks = [];
  for await (const chunk of process.stdin) {
    chunks.push(chunk);
  }
  return Buffer.concat(chunks).toString('utf8').trim();
}

/**
 * Extract the structured result from a delegate's output.
 * Looks for === DELEGATE RESULT === ... === END === markers.
 */
function parseDelegateOutput(output) {
  let summary = '';
  let files = [];

  const resultMatch = output.match(/=== DELEGATE RESULT ===\n([\s\S]*?)=== END ===/);
  if (resultMatch) {
    const section = resultMatch[1];
    const summaryMatch = section.match(/SUMMARY:\s*(.+)/);
    if (summaryMatch) summary = summaryMatch[1].trim();
    const filesMatch = section.match(/FILES:\s*(.+)/);
    if (filesMatch) {
      files = filesMatch[1].split(',').map(f => f.trim()).filter(Boolean);
    }
  }

  if (!summary) {
    // Fallback: use last non-empty text block
    const blocks = output.trim().split(/\n\n+/);
    summary = blocks[blocks.length - 1]?.slice(0, 500) || output.trim().slice(-500);
  }

  return { summary, files };
}

// ── Main ──────────────────────────────────────────────────────────────

async function main() {
  try {
    const input = JSON.parse(await readStdin());

    const cwd = input.cwd || process.cwd();
    const budgetMs = Math.min(input.budgetMs || 120000, 600_000);
    const task = input.task || 'No task specified';
    const context = input.context || '';
    const constraint = input.constraint || '';
    const outputDir = input.outputDir || '';

    log(`Delegate started: ${task.slice(0, 80)}`);
    log(`Budget: ${(budgetMs / 1000).toFixed(0)}s, CWD: ${cwd}`);

    // ── Build the sub-agent prompt ──────────────────────────────────
    const promptLines = [
      'You are a DELEGATED SUB-AGENT working on a specific task.',
      'You are an expert software engineer. Accomplish the task using your tools.',
      '',
      '## YOUR TASK',
      task,
      '',
      '## WORKING DIRECTORY',
      cwd,
      '',
      '## CONSTRAINTS',
      `- Budget: ${(budgetMs / 1000).toFixed(0)} seconds — manage your time wisely`,
      '- Focus only on the task above — do not expand scope',
      '- Read files first to understand existing code before making changes',
      constraint ? `- ${constraint}` : '',
      context ? `\n## CONTEXT\n${context}\n` : '',
      '',
      '## TOOLS',
      'You have all standard tools (read, bash, write, edit, grep, find, ls) plus workspace-aware tools:',
      '- `workspace_write` — Write file within workspace boundary',
      '- `workspace_append` — Append to file within workspace boundary',
      '- `workspace_exec` — Execute commands (preferred over bash for tracked work)',
      '- `workspace_validate` — Run validation commands and record results',
      '- `workspace_install_dep` — Install npm/pip/apt dependencies',
      '- `workspace_create_temp` — Create temp directory',
      '- `workspace_state` — Get workspace state summary',
      '- `workspace_memory_record/search/stats` — Memory system',
      '- `web_search` — Search web, arXiv, or Semantic Scholar',
      '',
      '## WORKFLOW',
      '1. Read relevant files to understand the existing codebase',
      '2. Plan your approach',
      '3. Implement the changes (use workspace_write for file creation)',
      '4. Verify your changes work if possible',
      '',
      '## REPORTING FORMAT',
      'End your response with exactly this section:',
      '',
      '=== DELEGATE RESULT ===',
      'SUMMARY: <one-line summary of what was accomplished>',
      'FILES: <comma-separated list of files created or modified>',
      '=== END ===',
    ].filter(Boolean).join('\n');

    // Write prompt to temp file to avoid shell escaping issues with long prompts
    const tmpDir = join(cwd, '.lemonharness', 'delegates', '_tmp');
    await mkdir(tmpDir, { recursive: true });
    const promptFile = join(tmpDir, `prompt-${Date.now().toString(36)}.txt`);
    await writeFile(promptFile, promptLines, 'utf8');

    log(`Prompt file: ${promptFile}`);

    // ── Spawn pi subprocess ────────────────────────────────────────
    // Use print mode: load LemonHarness extension and skills, but skip context files & themes
    const piArgs = [
      '-p',
      '--no-session',
      '--no-context-files',
      '--no-themes',
      `@${promptFile}`,
      'Complete the task above.',
    ];

    log(`Spawning: pi ${piArgs.slice(0, -1).join(' ')}`);

    const child = spawn('pi', piArgs, {
      cwd,
      stdio: ['ignore', 'pipe', 'pipe'],
      env: { ...process.env },
    });

    let stdout = '';
    let stderr = '';

    child.stdout.on('data', (d) => { stdout += d.toString(); });
    child.stderr.on('data', (d) => { stderr += d.toString(); });

    // ── Wait with timeout ──────────────────────────────────────────
    const result = await new Promise((resolvePromise) => {
      const timer = setTimeout(() => {
        child.kill('SIGTERM');
        // Give it a moment to flush output after SIGTERM
        setTimeout(() => {
          resolvePromise({ code: null, timedOut: true, output: stdout, error: stderr });
        }, 1000);
      }, budgetMs + 5000);

      child.on('close', (code) => {
        clearTimeout(timer);
        resolvePromise({ code, timedOut: false, output: stdout, error: stderr });
      });

      child.on('error', (err) => {
        clearTimeout(timer);
        resolvePromise({ code: null, timedOut: false, output: stdout, error: stderr + '\n' + err.message });
      });
    });

    log(`pi exited code=${result.code}, timedOut=${result.timedOut}, stdout=${stdout.length}B, stderr=${stderr.length}B`);

    // ── Parse results ──────────────────────────────────────────────
    const { summary, files } = parseDelegateOutput(result.output);
    const output = result.output.trim();

    // Strip the structured result block from the user-facing output
    const cleanOutput = output.replace(/=== DELEGATE RESULT ===[\s\S]*=== END ===/, '').trim()
      || summary;

    // Save output files if requested
    if (outputDir) {
      const outPath = resolve(cwd, outputDir);
      await mkdir(outPath, { recursive: true });
      await writeFile(join(outPath, 'output.txt'), result.output, 'utf8');
      await writeFile(join(outPath, 'summary.txt'), summary, 'utf8');
      if (files.length > 0) {
        await writeFile(join(outPath, 'files.json'), JSON.stringify(files, null, 2), 'utf8');
      }
    }

    // ── Emit result ────────────────────────────────────────────────
    if (result.timedOut) {
      log('Result: TIMED OUT');
      emitResult({
        success: false,
        summary: `Timed out after ${budgetMs}ms. ${summary.slice(0, 300)}`,
        files,
        output: cleanOutput.slice(0, 3000),
        toolCalls: 0,
      });
    } else if (result.code !== 0) {
      log('Result: FAILED');
      emitResult({
        success: false,
        summary: summary.slice(0, 500),
        files,
        output: cleanOutput.slice(0, 5000),
        error: stderr.slice(0, 300),
        toolCalls: 0,
      });
    } else {
      log('Result: COMPLETED');
      emitResult({
        success: true,
        summary: summary.slice(0, 500),
        files,
        output: cleanOutput,
        toolCalls: 0,
      });
    }

  } catch (err) {
    log(`Fatal error: ${err.message}`);
    if (err.stack) log(`Stack: ${err.stack.split('\n').slice(0, 3).join(' | ')}`);
    emitResult({ success: false, summary: `Delegate runner error: ${err.message}`, files: [] });
  }
}

main();
