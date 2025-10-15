#!/usr/bin/env python
import sys
import json
import librosa
import numpy as np

def estimate_bpm(y, sr):
    # librosa.beat.tempo gives tempo (bpm) estimate
    tempo = librosa.beat.tempo(y=y, sr=sr, aggregate=None)
    # use the median or mean of tempo candidates
    if tempo is None or len(tempo) == 0:
        return None
    return float(np.median(tempo))

def estimate_key(y, sr):
    # compute chroma (energy per pitch class)
    chroma = librosa.feature.chroma_cqt(y=y, sr=sr)
    # average over frames
    chroma_mean = np.mean(chroma, axis=1)
    # note names in chroma index order starting at C
    NOTES = ['C','C#','D','D#','E','F','F#','G','G#','A','A#','B']
    root_index = int(np.argmax(chroma_mean))
    root = NOTES[root_index]
    # rough major/minor detection: compare correlation with major/minor templates
    # templates (binary) for major and minor triads (relative to root)
    # Major: root, major third (+4), fifth (+7)
    # Minor: root, minor third (+3), fifth (+7)
    major_template = np.zeros(12); major_template[[0,4,7]] = 1
    minor_template = np.zeros(12); minor_template[[0,3,7]] = 1
    # rotate chroma_mean so root at index 0
    rot = np.roll(chroma_mean, -root_index)
    maj_score = np.dot(rot, major_template)
    min_score = np.dot(rot, minor_template)
    quality = "maj" if maj_score >= min_score else "min"
    return f"{root}{'' if quality=='maj' else 'm'}"

def main():
    if len(sys.argv) < 2:
        print(json.dumps({"error": "need input file"}))
        sys.exit(1)
    path = sys.argv[1]
    try:
        y, sr = librosa.load(path, sr=None, mono=True)
        bpm = estimate_bpm(y, sr)
        key = estimate_key(y, sr)
        res = {"bpm": bpm if bpm is not None else 0.0, "key": key}
        print(json.dumps(res))
    except Exception as e:
        print(json.dumps({"error": str(e)}))
        sys.exit(2)

if __name__ == "__main__":
    main()
