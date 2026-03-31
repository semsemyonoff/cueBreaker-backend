import logging
import os
import re
import glob
import shutil
import subprocess
import tempfile
import threading
from pathlib import Path
from flask import Flask, render_template, jsonify, request, send_file

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
)
log = logging.getLogger("cuebreaker")

app = Flask(__name__)

INPUT_DIR = os.environ.get("CUEBREAKER_INPUT_DIR", "/input")
OUTPUT_DIR = os.environ.get("CUEBREAKER_OUTPUT_DIR", "/output")

jobs = {}
job_lock = threading.Lock()

ENCODINGS = ["utf-8-sig", "utf-8", "cp1251", "cp1252", "shift_jis", "euc-kr", "latin-1"]


def read_cue(cue_path):
    """Read a CUE file, auto-detecting encoding. Returns UTF-8 string."""
    raw = open(cue_path, "rb").read()
    for enc in ENCODINGS:
        try:
            text = raw.decode(enc)
            if "TRACK" in text and "INDEX" in text:
                log.info("CUE %s detected as %s", cue_path, enc)
                return text
        except (UnicodeDecodeError, ValueError):
            continue
    log.warning("CUE %s: no encoding matched, falling back to latin-1", cue_path)
    return raw.decode("latin-1")


def make_utf8_cue(cue_path):
    """Create a temporary UTF-8 copy of a CUE file for external tools."""
    text = read_cue(cue_path)
    tmp = tempfile.NamedTemporaryFile(
        mode="w", suffix=".cue", encoding="utf-8", delete=False
    )
    tmp.write(text)
    tmp.close()
    return tmp.name


def parse_cue(cue_path):
    """Parse a CUE file and extract album info and track list."""
    content = read_cue(cue_path)

    album_info = {
        "performer": "",
        "title": "",
        "file": "",
        "genre": "",
        "date": "",
        "tracks": [],
    }

    m = re.search(r'^PERFORMER\s+"(.+?)"', content, re.MULTILINE)
    if m:
        album_info["performer"] = m.group(1)
    m = re.search(r'^TITLE\s+"(.+?)"', content, re.MULTILINE)
    if m:
        album_info["title"] = m.group(1)
    m = re.search(r'^FILE\s+"(.+?)"', content, re.MULTILINE)
    if m:
        album_info["file"] = m.group(1)
    m = re.search(r'^REM\s+GENRE\s+"?(.+?)"?\s*$', content, re.MULTILINE)
    if m:
        album_info["genre"] = m.group(1)
    m = re.search(r'^REM\s+DATE\s+(\S+)', content, re.MULTILINE)
    if m:
        album_info["date"] = m.group(1)

    track_blocks = re.split(r'^\s*TRACK\s+(\d+)\s+AUDIO', content, flags=re.MULTILINE)
    for i in range(1, len(track_blocks), 2):
        track_num = int(track_blocks[i])
        block = track_blocks[i + 1]

        track = {"number": track_num, "title": "", "performer": album_info["performer"]}

        m = re.search(r'TITLE\s+"(.+?)"', block)
        if m:
            track["title"] = m.group(1)
        m = re.search(r'PERFORMER\s+"(.+?)"', block)
        if m:
            track["performer"] = m.group(1)
        m = re.search(r'INDEX\s+01\s+(\d+:\d+:\d+)', block)
        if m:
            track["index"] = m.group(1)

        album_info["tracks"].append(track)

    return album_info


def cue_has_source_flac(cue_path, directory):
    """Check if a CUE file references a FLAC that actually exists (unsplit album)."""
    try:
        content = read_cue(cue_path)
    except Exception:
        return False
    # Find all FILE references in the CUE
    file_refs = re.findall(r'^FILE\s+"(.+?)"', content, re.MULTILINE)
    if not file_refs:
        return False
    # Single-file CUE (whole album in one FLAC) — the file must exist
    if len(file_refs) == 1:
        ref = file_refs[0]
        if ref.lower().endswith(".flac") or ref.lower().endswith(".wav"):
            return os.path.isfile(os.path.join(directory, ref))
    # Multi-file CUE (one FILE per track) — already split, skip
    return False


def check_output_status(rel_path, cue_path):
    """Check if output directory already has split files matching the CUE track count."""
    out_dir = os.path.join(OUTPUT_DIR, rel_path)
    if not os.path.isdir(out_dir):
        return {"done": False, "output_tracks": 0, "expected_tracks": 0}
    output_flacs = [f for f in os.listdir(out_dir) if f.lower().endswith(".flac")]
    try:
        cue_info = parse_cue(cue_path)
        expected = len(cue_info["tracks"])
    except Exception:
        expected = 0
    return {
        "done": len(output_flacs) > 0 and len(output_flacs) >= expected > 0,
        "output_tracks": len(output_flacs),
        "expected_tracks": expected,
    }


def find_cue_pairs(base_dir):
    """Find directories with CUE referencing an unsplit FLAC album."""
    results = []
    for root, dirs, files in os.walk(base_dir):
        cue_files = [f for f in files if f.lower().endswith(".cue")]
        if not cue_files:
            continue
        # Keep only CUEs that reference an existing single source file
        valid_cues = []
        for cf in cue_files:
            cue_path = os.path.join(root, cf)
            if cue_has_source_flac(cue_path, root):
                valid_cues.append(cf)
        if not valid_cues:
            continue
        flac_files = [f for f in files if f.lower().endswith(".flac")]
        rel_path = os.path.relpath(root, base_dir)
        first_cue = os.path.join(root, valid_cues[0])
        output_status = check_output_status(rel_path, first_cue)
        results.append(
            {
                "path": rel_path,
                "abs_path": root,
                "cue_files": sorted(valid_cues),
                "flac_files": sorted(flac_files),
                "split_done": output_status["done"],
                "output_tracks": output_status["output_tracks"],
            }
        )
    results.sort(key=lambda x: x["path"])
    return results


def find_cover(directory):
    """Find cover image in directory."""
    cover_patterns = [
        "cover.*", "Cover.*", "COVER.*",
        "folder.*", "Folder.*", "FOLDER.*",
        "front.*", "Front.*", "FRONT.*",
        "album.*", "Album.*",
    ]
    image_exts = {".jpg", ".jpeg", ".png", ".bmp", ".gif", ".webp"}
    for pattern in cover_patterns:
        for match in glob.glob(os.path.join(directory, pattern)):
            if Path(match).suffix.lower() in image_exts:
                return match
    for f in os.listdir(directory):
        if Path(f).suffix.lower() in image_exts:
            return os.path.join(directory, f)
    return None


def update_job(job_id, **kwargs):
    with job_lock:
        jobs[job_id].update(kwargs)


def tag_flac_with_cueprint(utf8_cue, flac_files, cue_info, job_id, total_steps=0):
    """Tag FLAC files using cueprint + metaflac."""
    total = len(flac_files)
    # If total_steps provided, tagging is the second half of the bar
    base = total_steps - total if total_steps else 0
    for idx, flac_path in enumerate(flac_files):
        track_num = idx + 1
        fname = os.path.basename(flac_path)
        log.info("[%s] Tagging track %d/%d: %s", job_id, track_num, total, fname)
        update_job(job_id, progress_current=base + track_num,
                   progress_total=total_steps or total,
                   progress_detail=f"Tagging: {fname}")

        tags = {}

        # Extract per-track tags via cueprint
        for fmt, tag_name in [
            ("%t", "TITLE"), ("%p", "ARTIST"), ("%n", "TRACKNUMBER"),
        ]:
            r = subprocess.run(
                ["cueprint", "-n", str(track_num), "-t", fmt, utf8_cue],
                capture_output=True, text=True, timeout=10,
            )
            if r.returncode == 0 and r.stdout.strip():
                tags[tag_name] = r.stdout.strip()

        # Album-level tags via cueprint
        for fmt, tag_name in [
            ("%T", "ALBUM"), ("%P", "ALBUMARTIST"),
        ]:
            r = subprocess.run(
                ["cueprint", "-d", fmt, utf8_cue],
                capture_output=True, text=True, timeout=10,
            )
            if r.returncode == 0 and r.stdout.strip():
                tags[tag_name] = r.stdout.strip()

        # Tags from our own parser (cueprint may not have genre/date)
        if cue_info["genre"]:
            tags.setdefault("GENRE", cue_info["genre"])
        if cue_info["date"]:
            tags.setdefault("DATE", cue_info["date"])
        tags["TRACKTOTAL"] = str(total)

        # Apply via metaflac
        metaflac_args = ["metaflac", "--remove-all-tags"]
        metaflac_args.append(flac_path)
        subprocess.run(metaflac_args, capture_output=True, timeout=10)

        set_args = ["metaflac"]
        for k, v in tags.items():
            set_args += [f"--set-tag={k}={v}"]
        set_args.append(flac_path)
        r = subprocess.run(set_args, capture_output=True, text=True, timeout=10)
        if r.returncode != 0:
            log.warning("[%s] metaflac failed for %s: %s", job_id, fname, r.stderr)
        else:
            log.info("[%s] Tagged %s: %s", job_id, fname, tags)


def split_cue(cue_path, source_dir, output_base, job_id):
    """Split a FLAC+CUE into individual tracks."""
    log.info("[%s] Starting split: cue=%s dir=%s", job_id, cue_path, source_dir)
    rel_path = os.path.relpath(source_dir, INPUT_DIR)
    out_dir = os.path.join(output_base, rel_path)
    os.makedirs(out_dir, exist_ok=True)
    log.info("[%s] Output dir: %s", job_id, out_dir)

    update_job(job_id, status="splitting", output_dir=out_dir,
               progress_current=0, progress_total=0, progress_detail="Preparing...")

    cue_info = parse_cue(cue_path)
    total_tracks = len(cue_info["tracks"])

    flac_file = None
    if cue_info["file"]:
        candidate = os.path.join(source_dir, cue_info["file"])
        if os.path.exists(candidate):
            flac_file = candidate
            log.info("[%s] FLAC from CUE: %s", job_id, flac_file)
    if not flac_file:
        for f in os.listdir(source_dir):
            if f.lower().endswith(".flac"):
                flac_file = os.path.join(source_dir, f)
                log.info("[%s] FLAC fallback: %s", job_id, flac_file)
                break

    if not flac_file:
        log.error("[%s] No FLAC file found in %s", job_id, source_dir)
        update_job(job_id, status="error", message="No FLAC file found")
        return

    utf8_cue = None
    try:
        utf8_cue = make_utf8_cue(cue_path)
        log.info("[%s] UTF-8 CUE temp: %s", job_id, utf8_cue)

        # cuebreakpoints
        update_job(job_id, progress_detail="Calculating breakpoints...")
        log.info("[%s] Running cuebreakpoints...", job_id)
        bp = subprocess.run(
            ["cuebreakpoints", utf8_cue],
            capture_output=True, text=True, timeout=30,
        )
        if bp.returncode != 0:
            log.error("[%s] cuebreakpoints failed (rc=%d): %s", job_id, bp.returncode, bp.stderr)
            update_job(job_id, status="error", message=f"cuebreakpoints failed: {bp.stderr}")
            return
        log.info("[%s] Breakpoints: %s", job_id, bp.stdout.strip().replace("\n", ", "))

        # shnsplit with real-time progress
        # Total steps = splitting tracks + tagging tracks
        total_steps = total_tracks * 2
        split_done = 0
        update_job(job_id, progress_detail="Splitting FLAC...",
                   progress_current=0, progress_total=total_steps)
        split_args = [
            "shnsplit", "-f", utf8_cue,
            "-O", "always",
            "-o", "flac", "-t", "%n - %t",
            "-d", out_dir, flac_file,
        ]
        log.info("[%s] Running shnsplit: %s", job_id, " ".join(split_args))
        proc = subprocess.Popen(
            split_args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
        )
        stderr_lines = []
        for line in proc.stderr:
            line = line.rstrip()
            stderr_lines.append(line)
            log.info("[%s] shnsplit: %s", job_id, line)
            # shnsplit outputs "Splitting [file] --> [output] : 100% OK" per track
            if "OK" in line or "-->" in line:
                split_done += 1
                track_name = ""
                m = re.search(r'-->\s+\[(.+?)\]', line)
                if m:
                    track_name = m.group(1)
                update_job(job_id, progress_current=min(split_done, total_tracks),
                           progress_detail=f"Splitting: {track_name or f'track {split_done}'}")
        proc.wait()
        if proc.stdout:
            stdout_data = proc.stdout.read()
            if stdout_data.strip():
                log.info("[%s] shnsplit stdout: %s", job_id, stdout_data.strip())

        if proc.returncode != 0:
            stderr_text = "\n".join(stderr_lines)
            log.error("[%s] shnsplit failed (rc=%d)", job_id, proc.returncode)
            update_job(job_id, status="error", message=f"shnsplit failed: {stderr_text}")
            return

        update_job(job_id, progress_current=total_tracks,
                   progress_detail="Splitting complete, tagging...")

        # Tag tracks
        log.info("[%s] Split done, applying tags...", job_id)
        update_job(job_id, status="tagging")
        flac_outputs = sorted(glob.glob(os.path.join(out_dir, "*.flac")))
        flac_outputs = [f for f in flac_outputs if not os.path.basename(f).startswith("00 -")]
        tag_flac_with_cueprint(utf8_cue, flac_outputs, cue_info, job_id, total_steps)

        # Remove pregap
        for f in os.listdir(out_dir):
            if f.startswith("00 -") and f.endswith(".flac"):
                log.info("[%s] Removing pregap: %s", job_id, f)
                os.remove(os.path.join(out_dir, f))

        # Copy cover
        update_job(job_id, progress_detail="Copying cover...")
        cover = find_cover(source_dir)
        if cover:
            log.info("[%s] Copying cover: %s", job_id, os.path.basename(cover))
            shutil.copy2(cover, out_dir)

        result_files = sorted(
            [f for f in os.listdir(out_dir) if f.lower().endswith(".flac")]
        )
        log.info("[%s] Done! %d tracks: %s", job_id, len(result_files), result_files)
        update_job(job_id, status="done", message="Split completed successfully",
                   result_files=result_files,
                   progress_current=total_steps, progress_total=total_steps,
                   progress_detail="Complete")

    except subprocess.TimeoutExpired:
        log.error("[%s] Process timed out", job_id)
        update_job(job_id, status="error", message="Process timed out")
    except Exception as e:
        log.exception("[%s] Unexpected error", job_id)
        update_job(job_id, status="error", message=str(e))
    finally:
        if utf8_cue and os.path.exists(utf8_cue):
            os.unlink(utf8_cue)


@app.route("/")
def index():
    return render_template("index.html")


@app.route("/api/scan")
def api_scan():
    log.info("Scanning %s for CUE+FLAC pairs...", INPUT_DIR)
    pairs = find_cue_pairs(INPUT_DIR)
    log.info("Found %d directories with CUE+FLAC", len(pairs))
    return jsonify(pairs)


@app.route("/api/search")
def api_search():
    q = request.args.get("q", "").lower()
    if not q:
        return jsonify([])
    pairs = find_cue_pairs(INPUT_DIR)
    filtered = [p for p in pairs if q in p["path"].lower()]
    return jsonify(filtered)


@app.route("/api/preview", methods=["POST"])
def api_preview():
    data = request.get_json()
    dir_path = data.get("path", "")
    cue_file = data.get("cue_file", "")

    abs_dir = os.path.join(INPUT_DIR, dir_path)
    cue_path = os.path.join(abs_dir, cue_file)

    if not os.path.isfile(cue_path):
        return jsonify({"error": "CUE file not found"}), 404

    real_cue = os.path.realpath(cue_path)
    real_input = os.path.realpath(INPUT_DIR)
    if not real_cue.startswith(real_input + "/"):
        return jsonify({"error": "Invalid path"}), 403

    info = parse_cue(cue_path)
    cover = find_cover(abs_dir)
    info["has_cover"] = cover is not None
    if cover:
        info["cover_name"] = os.path.basename(cover)

    output_status = check_output_status(dir_path, cue_path)
    info["split_done"] = output_status["done"]
    info["output_tracks"] = output_status["output_tracks"]

    return jsonify(info)


@app.route("/api/cover/<path:dir_path>")
def api_cover(dir_path):
    abs_dir = os.path.join(INPUT_DIR, dir_path)
    real_dir = os.path.realpath(abs_dir)
    real_input = os.path.realpath(INPUT_DIR)
    if not real_dir.startswith(real_input):
        return "", 403
    cover = find_cover(abs_dir)
    if not cover:
        return "", 404
    return send_file(cover)


@app.route("/api/split", methods=["POST"])
def api_split():
    data = request.get_json()
    dir_path = data.get("path", "")
    cue_file = data.get("cue_file", "")

    abs_dir = os.path.join(INPUT_DIR, dir_path)
    cue_path = os.path.join(abs_dir, cue_file)

    if not os.path.isfile(cue_path):
        return jsonify({"error": "CUE file not found"}), 404

    real_cue = os.path.realpath(cue_path)
    real_input = os.path.realpath(INPUT_DIR)
    if not real_cue.startswith(real_input + "/"):
        return jsonify({"error": "Invalid path"}), 403

    job_id = f"{dir_path}/{cue_file}"
    log.info("Split request: job_id=%s cue=%s", job_id, cue_path)
    with job_lock:
        if job_id in jobs and jobs[job_id]["status"] in ("splitting", "tagging"):
            log.warning("Job %s already in progress", job_id)
            return jsonify({"error": "Already in progress", "job_id": job_id}), 409
        jobs[job_id] = {"status": "queued", "message": "", "result_files": [],
                        "progress_current": 0, "progress_total": 0, "progress_detail": ""}

    thread = threading.Thread(
        target=split_cue, args=(cue_path, abs_dir, OUTPUT_DIR, job_id)
    )
    thread.daemon = True
    thread.start()

    return jsonify({"job_id": job_id, "status": "queued"})


@app.route("/api/status/<path:job_id>")
def api_status(job_id):
    with job_lock:
        job = jobs.get(job_id)
    if not job:
        return jsonify({"error": "Job not found"}), 404
    return jsonify(job)


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=5000)
