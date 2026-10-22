"""Normalize Markdown docs:
- remove emoji characters in common Unicode emoji ranges
- trim trailing whitespace
- create .bak backup before writing

Run from repo root: python3 scripts/normalize_docs.py

Walks every ``*.md`` file under the current directory, skipping any directory
whose path contains ``.git`` or ``node_modules`` as a substring. Characters in
``emoji_ranges`` are deleted, then trailing whitespace is stripped from every
line. A final newline is kept if the original had one, but multiple trailing
newlines collapse to one. Only files whose content actually changes are
rewritten; for each, the original text is first saved next to it as
``<name>.md.bak`` (an existing backup is overwritten). Prints the number of
files changed and emoji removed, lists up to 200 modified paths, and always
exits 0.
"""
import os
import sys

# Inclusive (start, end) Unicode code-point ranges treated as emoji: symbols and
# pictographs, emoticons, transport/map, misc symbols, supplemental symbols,
# regional-indicator letters (flags) and dingbats.
emoji_ranges=[
    (0x1F300,0x1F5FF),
    (0x1F600,0x1F64F),
    (0x1F680,0x1F6FF),
    (0x2600,0x26FF),
    (0x1F900,0x1F9FF),
    (0x1F1E6,0x1F1FF),
    (0x2700,0x27BF),
]

def is_emoji(ch):
    """Return True if ``ch`` falls in one of the ``emoji_ranges`` code-point ranges.

    Args:
        ch: A single-character string.

    Returns:
        bool: True when ``ord(ch)`` lies inside any inclusive (start, end)
        range in ``emoji_ranges``; False otherwise. The ranges cover the
        Miscellaneous Symbols and Dingbats blocks (U+2600-U+27BF), so
        non-emoji symbols such as a check mark or arrow-like dingbats are
        also matched.
    """
    o=ord(ch)
    for a,b in emoji_ranges:
        if a<=o<=b:
            return True
    return False

count=0
modified_files=[]
for dirpath,dirs,files in os.walk('.'):
    if '.git' in dirpath or 'node_modules' in dirpath:
        continue
    for fn in files:
        if not fn.endswith('.md'):
            continue
        fp=os.path.join(dirpath,fn)
        with open(fp,'r',encoding='utf-8') as f:
            txt=f.read()
        new_chars=[]
        changed=False
        for ch in txt:
            if is_emoji(ch):
                # remove emoji
                changed=True
                count+=1
            else:
                new_chars.append(ch)
        new=''.join(new_chars)
        # trim trailing whitespace on each line
        lines=new.splitlines()
        new2='\n'.join([ln.rstrip() for ln in lines])
        # preserve trailing newline if original had
        if txt.endswith('\n') and not new2.endswith('\n'):
            new2 += '\n'
        if new2!=txt:
            # backup
            bak=fp+'.bak'
            open(bak,'w',encoding='utf-8').write(txt)
            open(fp,'w',encoding='utf-8').write(new2)
            modified_files.append(fp)

print(f"Processed {len(modified_files)} files, removed {count} emoji characters.")
if modified_files:
    print('Modified files (first 200):')
    for p in modified_files[:200]:
        print(p)

sys.exit(0)
