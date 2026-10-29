#!/usr/bin/env python3
"""Write decision-path.svg from the rows below. Use --check to compare the file with this source."""
import argparse
import pathlib
import sys
from xml.sax.saxutils import escape

OUT = pathlib.Path(__file__).with_name('decision-path.svg')

# One row for each stage of the path in CONSTITUTION.md:
# (stage, lifecycle operations, what the application supplies, what the reference records).
ROWS = [
    ('Source', 'source, plan',
     ['Source authority, rule and data', 'content, independent expectations'],
     ['Source text and test cases for one', 'function, scope and date interval']),
    ('Proposal', 'propose',
     ['Author identity'],
     ['Snapshot: source, bundle, interval,', 'base snapshot and reason']),
    ('Qualification', 'qualify',
     ['The installed runner'],
     ['Report bound to the exact snapshot,', 'cases and build. Pass is not approval']),
    ('Approval', 'decide',
     ['Admission policy and reviewer', 'identity'],
     ['Decision bound to one passed report,', 'with its reason']),
    ('Dated activation', 'activate',
     [],
     ['Enable event for [from, until).', 'Overlaps fail unless masked']),
    ('Preparation', 'load',
     ['Worker that loads the revision'],
     ['Loaded bundle and engine build must', 'match the selected revision']),
    ('Execution', 'invoke',
     ['Authenticated caller, named inputs,', 'side effects of the answer'],
     ['Receipt: retained inputs, selected', 'revision, bundle and answer']),
    ('Feedback or correction', 'observe, checkpoint',
     ['Learning policy, corrected facts', 'as a new call with a relationship'],
     ['Observation and correction task;', 'a fix starts again at source']),
    ('Replay or restatement', 'replay, restate',
     ['Retention of receipts and inputs'],
     ['New receipt that names the earlier', 'receipt, reason and root']),
]

W = 960
ROW_H = 76
BOX_H = 54
TOP = 132
APP_X, APP_W = 24, 268
STAGE_X, STAGE_W = 344, 248
REF_X, REF_W = 668, 268
LOOP_X = 630

STYLE = """
.bg{fill:#F5F5F0}
.title{fill:#12181B;font:600 20px 'IBM Plex Sans',system-ui,sans-serif}
.head{fill:#50595E;font:600 12px 'IBM Plex Sans',system-ui,sans-serif;letter-spacing:.06em}
.stage{fill:#FFFFFF;stroke:#1E7A5C;stroke-width:2}
.stage-name{fill:#12181B;font:600 15px 'IBM Plex Sans',system-ui,sans-serif}
.op{fill:#1E7A5C;font:12px 'IBM Plex Mono',ui-monospace,monospace}
.app{fill:none;stroke:#50595E;stroke-width:1.5;stroke-dasharray:5 4}
.ref{fill:#E9EBE7;stroke:#D5D9D8;stroke-width:1}
.note{fill:#2C3438;font:12.5px 'IBM Plex Sans',system-ui,sans-serif}
.flow{stroke:#1E7A5C;stroke-width:2;fill:none}
.tie{stroke:#A9B0B3;stroke-width:1;fill:none}
.loop{stroke:#1E7A5C;stroke-width:1.5;fill:none;stroke-dasharray:6 4}
.arrow{fill:#1E7A5C}
.foot{fill:#50595E;font:12px 'IBM Plex Sans',system-ui,sans-serif}
@media (prefers-color-scheme:dark){
.bg{fill:#12181B}
.title,.stage-name{fill:#EEF0EC}
.head,.foot{fill:#A9B0B3}
.stage{fill:#12181B;stroke:#4FC79A}
.op{fill:#4FC79A}
.app{stroke:#A9B0B3}
.ref{fill:#2C3438;stroke:#50595E}
.note{fill:#E9EBE7}
.flow,.loop{stroke:#4FC79A}
.tie{stroke:#50595E}
.arrow{fill:#4FC79A}
}
"""


def text(x, y, cls, value, anchor='start'):
    return f'<text x="{x}" y="{y}" class="{cls}" text-anchor="{anchor}">{escape(value)}</text>'


def note(x, w, y, lines, cls):
    out = [f'<rect x="{x}" y="{y}" width="{w}" height="{BOX_H}" rx="6" class="{cls}"/>']
    first = y + BOX_H / 2 + 4.5 - 8 * (len(lines) - 1)
    out += [text(x + 12, first + 16 * i, 'note', line) for i, line in enumerate(lines)]
    return out


def render():
    h = TOP + ROW_H * len(ROWS) + 56
    out = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {h}" width="{W}" height="{h}" role="img" '
           'aria-labelledby="t d">',
           '<title id="t">Entelos decision path</title>',
           '<desc id="d">Nine stages from source to replay or restatement. The left column shows what the application '
           'supplies. The right column shows what the reference records.</desc>',
           f'<style>{STYLE}</style>',
           f'<rect width="{W}" height="{h}" class="bg"/>',
           '<defs><marker id="a" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" '
           'orient="auto"><path d="M0 0L10 5L0 10z" class="arrow"/></marker></defs>',
           text(24, 44, 'title', 'Decision path'),
           text(APP_X, 100, 'head', 'APPLICATION SUPPLIES'),
           text(STAGE_X, 100, 'head', 'STAGE AND LIFECYCLE OPERATION'),
           text(REF_X, 100, 'head', 'REFERENCE RECORDS AND CHECKS')]
    for i, (stage, ops, app, ref) in enumerate(ROWS):
        y = TOP + ROW_H * i
        mid = y + BOX_H / 2
        if app:
            out += note(APP_X, APP_W, y, app, 'app')
            out.append(f'<path d="M{APP_X + APP_W} {mid}H{STAGE_X}" class="tie"/>')
        out.append(f'<rect x="{STAGE_X}" y="{y}" width="{STAGE_W}" height="{BOX_H}" rx="8" class="stage"/>')
        out.append(text(STAGE_X + 14, y + 23, 'stage-name', stage))
        out.append(text(STAGE_X + 14, y + 42, 'op', ops))
        out.append(f'<path d="M{STAGE_X + STAGE_W} {mid}H{REF_X}" class="tie"/>')
        out += note(REF_X, REF_W, y, ref, 'ref')
        if i:
            cx = STAGE_X + STAGE_W / 2
            out.append(f'<path d="M{cx} {y - ROW_H + BOX_H}V{y - 1}" class="flow" marker-end="url(#a)"/>')
    # A correction makes a new revision, so the path starts again at source.
    fix = TOP + ROW_H * 7 + BOX_H / 2 + 10
    src = TOP + BOX_H / 2 + 10
    out.append(f'<path d="M{STAGE_X + STAGE_W} {fix}H{LOOP_X}V{src}H{STAGE_X + STAGE_W + 1}" class="loop" '
               'marker-end="url(#a)"/>')
    out.append(text(24, h - 22, 'foot',
                    'Dashed boxes: the application supplies these. Shaded boxes: the reference keeps these records. '
                    'Content hashes are identities, not signatures.'))
    out.append('</svg>')
    return '\n'.join(out) + '\n'


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--check', action='store_true', help='exit 1 if the SVG differs from this source')
    args = p.parse_args()
    svg = render()
    if args.check:
        if OUT.read_text() != svg:
            sys.exit(f'{OUT.name} differs from {pathlib.Path(__file__).name}; run it without --check')
        return
    OUT.write_text(svg)


if __name__ == '__main__':
    main()
