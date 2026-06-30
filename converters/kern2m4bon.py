#!/usr/bin/env python3
"""Convert **kern file to m4bon DSL.

Usage:
  kern2m4bon.py file.krn              # local file
  kern2m4bon.py 'search query'        # search + convert (auto-pick first)
  kern2m4bon.py --list 'search query' # list results, let user choose
  kern2m4bon.py -i N 'search query'   # pick result N from search
"""

import re
import sys
import urllib.request
import urllib.parse

TABLE = {
    'C': {'C':0,'D':0,'E':0,'F':0,'G':-1,'A':-1,'B':-1},
    'D': {'C':0,'D':0,'E':0,'F':0,'G':0,'A':-1,'B':-1},
    'E': {'C':0,'D':0,'E':0,'F':0,'G':0,'A':0,'B':-1},
    'F': {'C':0,'D':0,'E':0,'F':0,'G':0,'A':0,'B':0},
    'G': {'C':1,'D':0,'E':0,'F':0,'G':0,'A':0,'B':0},
    'A': {'C':1,'D':1,'E':0,'F':0,'G':0,'A':0,'B':0},
    'B': {'C':1,'D':1,'E':1,'F':0,'G':0,'A':0,'B':0},
}

FIFTHS_TO_KEY = {0:'C',1:'G',2:'D',3:'A',4:'E',5:'B',
                  -1:'F',-2:'&B',-3:'&E',-4:'&A',-5:'&D'}


def kern_octave(s):
    base = s.rstrip('#-')
    if base[0].islower():
        return 4 + len(base) - 1
    return 3 - (len(base) - 1)


def parse_kern_note(token):
    t = token.strip().strip('{}')
    if not t or t in ('*-',):
        return None
    m = re.match(r'(\d+)(\.?)([a-gA-G][#-]?[a-gA-G]?[#-]?)', t)
    if not m:
        return None
    dur = int(m.group(1))
    dotted = bool(m.group(2))
    pitch = m.group(3)
    # Duration in 16th-note units (LCM of all common note values)
    unit = {1: 16, 2: 8, 4: 4, 8: 2, 16: 1, 32: 0.5}.get(dur)
    if unit is None:
        return None
    dur_16ths = int(unit * 1.5) if dotted else unit
    letter = pitch[0].upper()
    return (letter,
            1 if '#' in pitch else (-1 if '-' in pitch else 0),
            kern_octave(pitch), dur_16ths)


def note_to_m4bon(letter, accidental, octave, prev, key_sig):
    s = ''
    ref_l, ref_o = prev
    if ref_l is None:
        ref_l, ref_o = 'C', 4
    delta = octave - (ref_o + TABLE[ref_l][letter])
    if delta > 0:
        s += '^' * delta
    elif delta < 0:
        s += '/' * -delta
    if accidental != key_sig.get(letter, 0):
        s += '#' if accidental == 1 else '&'
    prev[0], prev[1] = letter, octave
    return s + letter.lower()


def pack_measure(events, beats_total, ticks_per_beat, key_sig, prev):
    if not events:
        return ' '.join(['-'] * beats_total)
    onsets = {e[3]: (e[0], e[1], e[2]) for e in events}
    tokens = []
    b = 0
    while b < beats_total:
        start = b * ticks_per_beat
        end = (b + 1) * ticks_per_beat
        beat_onsets = [(t, onsets[t]) for t in onsets if start <= t < end]
        if not beat_onsets:
            tokens.append('-')
            b += 1
            continue
        first_t = beat_onsets[0][0]
        first_l, first_a, first_o = onsets[first_t]
        first_dur = sum(e[4] for e in events if e[3] == first_t)
        end_tick = first_t + first_dur
        span = max(1, (end_tick - start + ticks_per_beat - 1) // ticks_per_beat)
        if span > 1:
            span_ticks = span * ticks_per_beat
            other_onsets = [t for t in onsets if start < t < start + span_ticks]
            if not other_onsets:
                s = note_to_m4bon(first_l, first_a, first_o, prev, key_sig)
                tokens.append(s)
                for _ in range(span - 1):
                    tokens.append('-')
                b += span
            else:
                chars = []
                for t in range(start, start + span_ticks):
                    if t in onsets:
                        l, a, o = onsets[t]
                        chars.append(note_to_m4bon(l, a, o, prev, key_sig))
                    else:
                        chars.append('-')
                tokens.append(str(span) + ''.join(chars))
                b += span
        else:
            beat_onsets.sort()
            chars = []
            for t, (l, a, o) in beat_onsets:
                chars.append(note_to_m4bon(l, a, o, prev, key_sig))
            tokens.append(''.join(chars))
            b += 1
    return ' '.join(tokens)


def kern_to_m4bon(kern_text):
    lines = kern_text.strip().split('\n')
    meter = '4/4'
    key_sig = {}
    measures = []
    pickup = []
    current = []
    seen_barline = False

    for line in lines:
        line = line.strip()
        if not line or line.startswith('!!!') or line.startswith('*I'):
            continue
        if line.startswith('*M'):
            meter = line[2:]
            continue
        if line.startswith('*k['):
            key_sig = {}
            for a in re.findall(r'([a-g][#-])', line[3:-1]):
                key_sig[a[0].upper()] = 1 if a[1] == '#' else -1
            continue
        if line.startswith('*'):
            continue
        if line.startswith('='):
            if current:
                if not seen_barline:
                    pickup = current
                else:
                    measures.append(current)
                current = []
                seen_barline = True
            continue
        note = parse_kern_note(line)
        if note:
            current.append(note)
    if current:
        measures.append(current)

    mm = re.match(r'(\d+)/(\d+)', meter)
    beats_per_measure = int(mm.group(1)) if mm else 3
    ticks_per_beat = 4   # 1 quarter = 4 sixteenths

    fifths = sum(1 for v in key_sig.values() if v == 1) - sum(1 for v in key_sig.values() if v == -1)
    result = [f'K{FIFTHS_TO_KEY.get(fifths, "C")} M{meter}']
    prev = [None, None]

    pickup_ticks = 0
    if pickup:
        tick = 0
        events = []
        for l, a, o, d in pickup:
            events.append((l, a, o, tick, d))
            tick += d
        pickup_ticks = tick
        pickup_beats = max(1, (pickup_ticks + ticks_per_beat - 1) // ticks_per_beat)
        result.append(pack_measure(events, pickup_beats, ticks_per_beat, key_sig, prev))

    remaining = list(measures)
    if pickup and remaining:
        last = remaining.pop()
        tick = 0
        events = []
        for l, a, o, d in last:
            events.append((l, a, o, tick, d))
            tick += d
        full_ticks = beats_per_measure * ticks_per_beat
        adj_ticks = full_ticks - pickup_ticks
        adj_beats = max(1, adj_ticks // ticks_per_beat)
        if adj_ticks > 0 and adj_beats > 0:
            result.append(pack_measure(events, adj_beats, ticks_per_beat, key_sig, prev))
        else:
            remaining.append(last)

    for measure in remaining:
        tick = 0
        events = []
        for l, a, o, d in measure:
            events.append((l, a, o, tick, d))
            tick += d
        result.append(pack_measure(events, beats_per_measure, ticks_per_beat, key_sig, prev))

    return '\n'.join(result)


def search_kern(query):
    url = f'https://kern.humdrum.org/cgi-bin/kssearch?s=t&keyword={urllib.parse.quote(query)}'
    html = urllib.request.urlopen(url).read().decode('utf-8')
    results = []
    seen = set()
    # Find all result rows: extract location+file from ksdata links,
    # then walk forward to the title link.
    for m in re.finditer(r'ksdata\?location=([^&]+)&file=([^&]+)&format=kern', html):
        loc = urllib.parse.unquote(m.group(1))
        fn = urllib.parse.unquote(m.group(2))
        key = (loc, fn)
        if key in seen:
            continue
        seen.add(key)
        # Walk forward to find the corresponding info link with title
        chunk = html[m.start():m.start() + 2500]
        tm = re.search(r'format=info>([^<]+)</a>', chunk)
        title = tm.group(1) if tm else fn
        results.append((title, loc, fn))
    return results


def fetch_kern(loc, fn):
    url = f'https://kern.humdrum.org/cgi-bin/ksdata?location={urllib.parse.quote(loc)}&file={fn}&format=kern'
    return urllib.request.urlopen(url).read().decode('utf-8')


if __name__ == '__main__':
    argv = sys.argv[1:]

    if not argv:
        print(__doc__, file=sys.stderr)
        sys.exit(1)

    if argv[0].endswith('.krn'):
        with open(argv[0]) as f:
            print(kern_to_m4bon(f.read()))
        sys.exit(0)

    list_only = False
    index = 0
    args = []
    for a in argv:
        if a == '--list':
            list_only = True
        elif a.startswith('-i') and len(a) > 2:
            try:
                index = int(a[2:])
            except ValueError:
                pass
        elif a.startswith('--index='):
            try:
                index = int(a.split('=', 1)[1])
            except ValueError:
                pass
        else:
            args.append(a)

    query = ' '.join(args)
    results = search_kern(query)

    if not results:
        print(f"No results for '{query}'", file=sys.stderr)
        sys.exit(1)

    if list_only or index >= len(results):
        print(f"Found {len(results)} results for '{query}':\n")
        for i, (title, loc, fn) in enumerate(results):
            print(f"  [{i}] {title}")
            print(f"       {loc}/{fn}")
        if list_only:
            sys.exit(0)
        print(f"\nUse -i<N> to select one (e.g. -i0), or --list to show all")
        sys.exit(0)

    title, loc, fn = results[index]
    print(f"# {title}", file=sys.stderr)
    print(f"# {loc}/{fn}", file=sys.stderr)
    kern_text = fetch_kern(loc, fn)
    print(kern_to_m4bon(kern_text))
