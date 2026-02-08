# -*- coding: utf-8 -*-
"""
ObfusPS-Tool - CLI interface for ObfusPS (PowerShell obfuscation).
Runs the Go engine via 'go run ./cmd/obfusps' (from project root) or 'obfusps' from PATH.
"""
import sys
import os
import datetime
import shlex
import shutil
import subprocess

try:
    import colorama
except ImportError:
    print("[!] colorama is required: pip install colorama")
    sys.exit(1)

try:
    import ctypes
except ImportError:
    ctypes = None

try:
    import tkinter as tk
    from tkinter import filedialog
except ImportError:
    tk = None
    filedialog = None

from colorama import init
init(autoreset=True)

# Ensure UTF-8 output on Windows (banner uses Unicode box/block characters)
if sys.platform == 'win32':
    try:
        sys.stdout.reconfigure(encoding='utf-8', errors='replace')
        sys.stderr.reconfigure(encoding='utf-8', errors='replace')
    except Exception:
        pass

color = colorama.Fore
red = color.RED
white = color.WHITE
reset = color.RESET
BEFORE = f'{red}[{white}'
AFTER = f'{red}]'
INPUT = f'{BEFORE}>{AFTER} |'
INFO = f'{BEFORE}!{AFTER} |'
ERROR = f'{BEFORE}x{AFTER} |'
ADD = f'{BEFORE}+{AFTER} |'
WAIT = f'{BEFORE}~{AFTER} |'

# Maximum input file size (50 MB) — safety limit
MAX_INPUT_SIZE = 50 * 1024 * 1024

# Valid profiles (must match Go engine)
VALID_PROFILES = ('safe', 'light', 'balanced', 'heavy', 'stealth', 'paranoid', 'redteam', 'blueteam', 'size', 'dev')

github = 'github.com/BenzoXdev'
telegram = 't.me/benzoXdev'
instagram = 'instagram.com/just._.amar_x1'
by = 'BenzoXdev'

folder = 'ObfusPS'
output_folder_1 = os.path.join(folder, 'Script-Obfuscate')
script_folder = os.path.join(folder, 'Script')

BANNER = r'''
 %s                                                                                                  

                            ▒█████   ▄▄▄▄     █████▒█    ██   ██████  ██▓███    ██████ 
                            ▒██▒  ██▒▓█████▄ ▓██   ▒ ██  ▓██▒▒██    ▒ ▓██░  ██▒▒██    ▒ 
                            ▒██░  ██▒▒██▒ ▄██▒████ ░▓██  ▒██░░ ▓██▄   ▓██░ ██▓▒░ ▓██▄   
                            ▒██   ██░▒██░█▀  ░▓█▒  ░▓▓█  ░██░  ▒   ██▒▒██▄█▓▒ ▒  ▒   ██▒
                            ░ ████▓▒░░▓█  ▀█▓░▒█░   ▒▒█████▓ ▒██████▒▒▒██▒ ░  ░▒██████▒▒
                            ░ ▒░▒░▒░ ░▒▓███▀▒ ▒ ░   ░▒▓▒ ▒ ▒ ▒ ▒▓▒ ▒ ░▒▓▒░ ░  ░▒ ▒▓▒ ▒ ░
                            ░ ▒ ▒░ ▒░▒   ░  ░     ░░▒░ ░ ░ ░ ░▒  ░ ░░▒ ░     ░ ░▒  ░ ░
                            ░ ░ ░ ▒   ░    ░  ░ ░    ░░░ ░ ░ ░  ░  ░  ░░       ░  ░  ░  
                                ░ ░   ░                ░           ░                 ░  
                                        ░                                            
%s                                              ObfusPS  |  v.1.2.0

%s                                              GitHub : %s

                                                  ╔═════════════════╗
                                                  ║ %sObfuscator Tool%s ║
                                                  ╚═════════════════╝

%s[%s>%s]%s Telegram : %s%s
%s[%s>%s]%s Instagram : %s%s
''' % (red, white, white, github, red, white, red, white, red, reset, white, telegram, red, white, red, reset, white, instagram)


def current_time_hour():
    return datetime.datetime.now().strftime('%H:%M:%S')


def Title(title):
    if sys.platform.startswith('win') and ctypes:
        try:
            ctypes.windll.kernel32.SetConsoleTitleW(f'ObfusPS - Obfuscator Tool | {title}')
        except Exception:
            pass
    else:
        # Works on Linux and macOS
        try:
            sys.stdout.write(f'\033]2;ObfusPS - Obfuscator Tool | {title}\a')
            sys.stdout.flush()
        except Exception:
            pass


def Clear():
    if sys.platform.startswith('win'):
        subprocess.run(['cmd', '/c', 'cls'], shell=False)
    else:
        subprocess.run(['clear'], shell=False, stderr=subprocess.DEVNULL)


def sanitize_path(path):
    """Validate and sanitize a file path. Returns absolute path or None if invalid."""
    path = path.strip().strip('"').strip("'")
    if not path:
        return None
    try:
        abs_path = os.path.abspath(os.path.normpath(path))
    except (ValueError, OSError):
        return None
    # Reject paths with null bytes
    if '\x00' in abs_path:
        return None
    return abs_path


def validate_ps1_file(path, ts):
    """Validate that a file is a valid PowerShell script. Returns True if valid."""
    if not os.path.isfile(path):
        print(f'{ts} {ERROR} File not found: {path}')
        return False
    lower = path.lower()
    if not (lower.endswith('.ps1') or lower.endswith('.psm1')):
        print(f'{ts} {ERROR} Not a PowerShell file (.ps1/.psm1): {path}')
        return False
    try:
        file_size = os.path.getsize(path)
    except OSError:
        print(f'{ts} {ERROR} Cannot read file size: {path}')
        return False
    if file_size == 0:
        print(f'{ts} {ERROR} File is empty: {path}')
        return False
    if file_size > MAX_INPUT_SIZE:
        print(f'{ts} {ERROR} File too large ({file_size} bytes, max {MAX_INPUT_SIZE}): {path}')
        return False
    return True


def get_script_dir():
    """Project/script directory (for 'go run' or PATH)."""
    if getattr(sys, 'frozen', False):
        return os.path.dirname(sys.executable)
    return os.path.dirname(os.path.abspath(__file__))


def find_obfusps_runner():
    """
    Find how to run the obfusps engine.
    Priority: 1) compiled binary in same dir, 2) obfusps in PATH, 3) go run.
    Returns ('binary', path) or ('go_run', project_root) or None.
    """
    script_dir = get_script_dir()
    # 1) Check for compiled binary in same directory (fastest)
    for name in ('ObfusPS.exe', 'obfusps.exe') if sys.platform == 'win32' else ('obfusps',):
        exe_path = os.path.join(script_dir, name)
        if os.path.isfile(exe_path):
            return ('binary', os.path.abspath(exe_path))
    # 2) Check PATH
    obfusps_path = shutil.which('obfusps')
    if obfusps_path:
        return ('binary', os.path.abspath(obfusps_path))
    # 3) Fallback: go run (slower, requires Go SDK)
    go_mod = os.path.join(script_dir, 'go.mod')
    cmd_main = os.path.join(script_dir, 'cmd', 'obfusps', 'main.go')
    if os.path.isfile(go_mod) and os.path.isfile(cmd_main) and shutil.which('go'):
        return ('go_run', script_dir)
    return None


def ChoosePowerShellFile():
    """Return a list of sanitized PowerShell (.ps1/.psm1) file paths. Opens file picker first."""
    ts = BEFORE + current_time_hour() + AFTER
    chosen = []
    if tk and filedialog:
        root = None
        try:
            root = tk.Tk()
            root.withdraw()
            root.attributes('-topmost', True)
            if sys.platform == 'win32':
                try:
                    root.overrideredirect(False)
                    root.update_idletasks()
                except Exception:
                    pass
            ps_files = filedialog.askopenfilenames(
                parent=root,
                title='ObfusPS - Choose one or more PowerShell files (.ps1 / .psm1)',
                filetypes=[
                    ('PowerShell files', '*.ps1;*.psm1'),
                    ('All files', '*.*')
                ],
                initialdir=os.path.expanduser('~') or os.getcwd()
            )
            root.destroy()
            root = None
            if ps_files:
                for p in ps_files:
                    safe = sanitize_path(p)
                    if safe:
                        chosen.append(safe)
                if chosen:
                    print(f'{ts} {INPUT} Selected files -> {reset}')
                    for p in chosen:
                        print(f'{ts} {ADD} {white}{p}{reset}')
                    return chosen
        except Exception as e:
            print(f'{ts} {ERROR} File picker error: {e}')
            if root:
                try:
                    root.destroy()
                except Exception:
                    pass
    print(f'{ts} {INFO} No selection. Enter paths manually?')
    try:
        rep = input(f'{ts} {INPUT} Paths (comma-separated) or Enter to cancel -> {reset}').strip()
    except KeyboardInterrupt:
        print()
        return []
    if rep:
        for raw in rep.split(','):
            safe = sanitize_path(raw)
            if safe:
                chosen.append(safe)
                print(f'{ts} {ADD} {white}{safe}{reset}')
            elif raw.strip():
                print(f'{ts} {ERROR} Invalid path: {raw.strip()}')
    return chosen


def _run_recommend(runner, file_path, ts):
    """Run -recommend on a single file and print the analysis."""
    try:
        kwargs = {
            'capture_output': True,
            'encoding': 'utf-8',
            'errors': 'replace',
        }
        if sys.platform == 'win32':
            flags = getattr(subprocess, 'CREATE_NO_WINDOW', 0)
            if flags:
                kwargs['creationflags'] = flags
        mode, value = runner
        base_cmd = ['-i', file_path, '-recommend']
        if mode == 'go_run':
            cmd = ['go', 'run', './cmd/obfusps'] + base_cmd
            kwargs['cwd'] = value
        else:
            cmd = [value] + base_cmd
        r = subprocess.run(cmd, **kwargs)
        if r.stderr:
            for line in r.stderr.strip().splitlines():
                line = line.strip()
                if line:
                    print(f'{ts} {INFO} {line}')
    except Exception as e:
        print(f'{ts} {ERROR} Recommend error: {e}')


def ObfusPS_Tool():
    Clear()
    Title(f'By: {by}')
    print(BANNER)

    runner = find_obfusps_runner()
    if not runner:
        ts = BEFORE + current_time_hour() + AFTER
        print(f'{ts} {ERROR} obfusps not found. Install Go and run from project root, or run: go install github.com/benzoXdev/obfusps/cmd/obfusps@latest')
        try:
            input(f'{ts} {INPUT} Press Enter to continue.. ')
        except KeyboardInterrupt:
            print()
        return

    files_ps1 = ChoosePowerShellFile()
    if not files_ps1:
        ts = BEFORE + current_time_hour() + AFTER
        print(f'{ts} {ERROR} No file selected.')
        return

    ts = BEFORE + current_time_hour() + AFTER
    valid_files = []
    for fp in files_ps1:
        if validate_ps1_file(fp, ts):
            valid_files.append(fp)
    if not valid_files:
        print(f'{ts} {ERROR} No valid file.')
        return

    # --- Mode menu ---
    green = color.GREEN
    cyan = color.CYAN
    yellow = color.YELLOW
    print(f'''
    {cyan}╔══ Mode ══════════════════════════════════════════════════╗{reset}
    {cyan}║{reset}  {green}[{white}0{green}] {white}AUTO        {yellow}(smart — auto-detect best settings)    {cyan}║{reset}
    {cyan}║{reset}  {red}[{white}M{red}] {white}MANUAL      {reset}(choose level + profile manually)    {cyan}║{reset}
    {cyan}║{reset}  {red}[{white}C{red}] {white}COMMAND     {reset}(type raw flags / full control)      {cyan}║{reset}
    {cyan}║{reset}  {red}[{white}R{red}] {white}RECOMMEND   {reset}(analyze script, no obfuscation)     {cyan}║{reset}
    {cyan}╚═════════════════════════════════════════════════════════╝{reset}
    ''')

    try:
        mode_input = input(f'{ts} {INPUT} Mode (0=Auto/M=Manual/C=Command/R=Recommend, default=0) -> {reset}').strip().lower()
    except KeyboardInterrupt:
        print()
        return

    use_auto = mode_input in ('0', '', 'auto', 'a')
    recommend_only = mode_input in ('r', 'recommend')
    command_mode = mode_input in ('c', 'cmd', 'command')

    obfuscation_force = 3
    profile = 'balanced'
    use_ast = True
    do_validate = False
    custom_flags = []

    if command_mode:
        # COMMAND mode: let the user type raw flags
        print(f'{ts} {INFO} {green}COMMAND mode{reset}: type raw ObfusPS flags. {yellow}-i{reset} and {yellow}-o{reset} are auto-filled.')
        print(f'''
    {cyan}╔══ Available Flags ═══════════════════════════════════════════════════════════════╗{reset}
    {cyan}║{reset}                                                                                {cyan}║{reset}
    {cyan}║{reset}  {yellow}── Core ──────────────────────────────────────────────────────────────────{reset}  {cyan}║{reset}
    {cyan}║{reset}  {white}-level 1..5{reset}              Obfuscation level (1=weak .. 5=extreme)           {cyan}║{reset}
    {cyan}║{reset}  {white}-profile <name>{reset}          Preset: safe|light|balanced|heavy|stealth|       {cyan}║{reset}
    {cyan}║{reset}                            paranoid|redteam|blueteam|size|dev              {cyan}║{reset}
    {cyan}║{reset}  {white}-layers <L1,L2,...>{reset}      Layers: AST,Flow,Encoding,Runtime                {cyan}║{reset}
    {cyan}║{reset}  {white}-pipeline <t1,t2,...>{reset}    Custom: iden,strenc,stringdict,numenc,fmt,cf,dead {cyan}║{reset}
    {cyan}║{reset}                                                                                {cyan}║{reset}
    {cyan}║{reset}  {yellow}── Encoding Transforms ───────────────────────────────────────────────{reset}  {cyan}║{reset}
    {cyan}║{reset}  {white}-strenc off|xor|rc4{reset}     String encryption mode                            {cyan}║{reset}
    {cyan}║{reset}  {white}-strkey <hex>{reset}            Hex key for -strenc (e.g. a1b2c3d4)              {cyan}║{reset}
    {cyan}║{reset}  {white}-stringdict 0..100{reset}      String tokenization percentage                    {cyan}║{reset}
    {cyan}║{reset}  {white}-numenc{reset}                  Enable number encoding                            {cyan}║{reset}
    {cyan}║{reset}  {white}-iden obf|keep{reset}           Identifier morphing (obf=rename)                  {cyan}║{reset}
    {cyan}║{reset}  {white}-fmt off|jitter{reset}          Format jitter (whitespace randomization)          {cyan}║{reset}
    {cyan}║{reset}                                                                                {cyan}║{reset}
    {cyan}║{reset}  {yellow}── Flow Transforms ───────────────────────────────────────────────────{reset}  {cyan}║{reset}
    {cyan}║{reset}  {white}-cf-opaque{reset}               Opaque predicate wrapper                          {cyan}║{reset}
    {cyan}║{reset}  {white}-cf-shuffle{reset}              Shuffle function blocks                            {cyan}║{reset}
    {cyan}║{reset}  {white}-deadcode 0..100{reset}         Dead-code injection probability                   {cyan}║{reset}
    {cyan}║{reset}  {white}-flow-unsafe{reset}             Disable FlowSafeMode (redteam/paranoid)           {cyan}║{reset}
    {cyan}║{reset}                                                                                {cyan}║{reset}
    {cyan}║{reset}  {yellow}── Level 5 / Fragmentation ───────────────────────────────────────────{reset}  {cyan}║{reset}
    {cyan}║{reset}  {white}-frag profile=<p>{reset}        tight|medium|loose|pro                             {cyan}║{reset}
    {cyan}║{reset}  {white}-minfrag N{reset}               Minimum fragment size (default 10)                 {cyan}║{reset}
    {cyan}║{reset}  {white}-maxfrag N{reset}               Maximum fragment size (default 20)                 {cyan}║{reset}
    {cyan}║{reset}  {white}-no-integrity{reset}            Disable integrity check (default=true)             {cyan}║{reset}
    {cyan}║{reset}  {white}-no-integrity=false{reset}      Enable integrity check                             {cyan}║{reset}
    {cyan}║{reset}                                                                                {cyan}║{reset}
    {cyan}║{reset}  {yellow}── Smart / Auto ──────────────────────────────────────────────────────{reset}  {cyan}║{reset}
    {cyan}║{reset}  {white}-auto{reset}                    Auto-detect best profile/level/flags               {cyan}║{reset}
    {cyan}║{reset}  {white}-auto-retry{reset}              Auto-retry with safer settings on failure          {cyan}║{reset}
    {cyan}║{reset}  {white}-recommend{reset}               Analyze only, print recommendations                {cyan}║{reset}
    {cyan}║{reset}                                                                                {cyan}║{reset}
    {cyan}║{reset}  {yellow}── Validation ────────────────────────────────────────────────────────{reset}  {cyan}║{reset}
    {cyan}║{reset}  {white}-validate{reset}                Compare original vs obfuscated output              {cyan}║{reset}
    {cyan}║{reset}  {white}-validate-args "..."{reset}     Args passed to script during validation            {cyan}║{reset}
    {cyan}║{reset}  {white}-validate-stderr <m>{reset}     strict|ignore (default=strict)                     {cyan}║{reset}
    {cyan}║{reset}  {white}-validate-timeout N{reset}      Timeout in seconds (default=30)                    {cyan}║{reset}
    {cyan}║{reset}                                                                                {cyan}║{reset}
    {cyan}║{reset}  {yellow}── AST / Protection ──────────────────────────────────────────────────{reset}  {cyan}║{reset}
    {cyan}║{reset}  {white}-use-ast{reset}                 Use native PowerShell AST (requires pwsh)          {cyan}║{reset}
    {cyan}║{reset}  {white}-context-aware{reset}           Skip strenc for IEX/Add-Type/ScriptBlock           {cyan}║{reset}
    {cyan}║{reset}  {white}-module-aware{reset}            Protect Import-Module, dot-sourcing, exports       {cyan}║{reset}
    {cyan}║{reset}  {white}-anti-reverse{reset}            Inject anti-debug/sandbox checks                   {cyan}║{reset}
    {cyan}║{reset}                                                                                {cyan}║{reset}
    {cyan}║{reset}  {yellow}── Output / Misc ─────────────────────────────────────────────────────{reset}  {cyan}║{reset}
    {cyan}║{reset}  {white}-seed N{reset}                  RNG seed (0=random, N=reproducible)                {cyan}║{reset}
    {cyan}║{reset}  {white}-fuzz N{reset}                  Generate N fuzzed variants                         {cyan}║{reset}
    {cyan}║{reset}  {white}-report{reset}                  Emit obfuscation report                            {cyan}║{reset}
    {cyan}║{reset}  {white}-dry-run{reset}                 Analyze only, no output                            {cyan}║{reset}
    {cyan}║{reset}  {white}-noexec{reset}                  Payload only, no Invoke-Expression                 {cyan}║{reset}
    {cyan}║{reset}  {white}-q{reset}                       Quiet mode (no banner)                             {cyan}║{reset}
    {cyan}║{reset}  {white}-log file.log{reset}            Write debug log to file                            {cyan}║{reset}
    {cyan}║{reset}                                                                                {cyan}║{reset}
    {cyan}╠══ Examples ════════════════════════════════════════════════════════════════════════╣{reset}
    {cyan}║{reset}                                                                                {cyan}║{reset}
    {cyan}║{reset}  {green}-level 5 -profile redteam -anti-reverse -validate{reset}                         {cyan}║{reset}
    {cyan}║{reset}  {green}-level 3 -profile balanced -strenc xor -strkey a1b2c3d4 -numenc{reset}           {cyan}║{reset}
    {cyan}║{reset}  {green}-auto -auto-retry -validate -validate-stderr ignore{reset}                       {cyan}║{reset}
    {cyan}║{reset}  {green}-layers AST,Flow,Encoding,Runtime -report{reset}                                 {cyan}║{reset}
    {cyan}║{reset}  {green}-level 4 -pipeline iden,strenc,numenc,fmt -iden obf -strenc rc4 -strkey 0011{reset}{cyan}║{reset}
    {cyan}║{reset}  {green}-level 5 -frag profile=pro -minfrag 5 -maxfrag 14 -seed 42{reset}               {cyan}║{reset}
    {cyan}║{reset}  {green}-profile heavy -context-aware -use-ast -module-aware -validate{reset}            {cyan}║{reset}
    {cyan}║{reset}  {green}-level 3 -profile safe -seed 12345 -validate -report{reset}                     {cyan}║{reset}
    {cyan}║{reset}  {green}-level 5 -profile paranoid -flow-unsafe -anti-reverse -fuzz 3{reset}             {cyan}║{reset}
    {cyan}║{reset}  {green}-dry-run -level 4 -profile stealth{reset}                                       {cyan}║{reset}
    {cyan}║{reset}                                                                                {cyan}║{reset}
    {cyan}╚════════════════════════════════════════════════════════════════════════════════════╝{reset}
        ''')
        try:
            raw_flags = input(f'{ts} {INPUT} Flags -> {reset}').strip()
        except KeyboardInterrupt:
            print()
            return
        if raw_flags:
            # Parse the raw string into a list of arguments (respecting quotes)
            try:
                custom_flags = shlex.split(raw_flags)
            except ValueError:
                # Fallback: simple split
                custom_flags = raw_flags.split()
        # Remove any -i / -o that the user may have typed (we fill those automatically)
        filtered = []
        skip_next = False
        for i, f in enumerate(custom_flags):
            if skip_next:
                skip_next = False
                continue
            if f in ('-i', '-o') and i + 1 < len(custom_flags):
                skip_next = True
                print(f'{ts} {INFO} {yellow}Ignoring {f} (auto-filled){reset}')
                continue
            filtered.append(f)
        custom_flags = filtered
        print(f'{ts} {INFO} Flags: {white}{" ".join(custom_flags) if custom_flags else "(encoding only)"}{reset}')

    if recommend_only:
        # Recommend mode: just run -recommend on each file
        for fp in valid_files:
            _run_recommend(runner, fp, ts)
        try:
            input(f'{ts} {INPUT} Press Enter to continue.. ')
        except KeyboardInterrupt:
            print()
        return

    if use_auto:
        print(f'{ts} {INFO} {green}AUTO mode{reset}: engine will analyze script and auto-select best settings.')
        do_validate = True
        use_ast = True
    elif command_mode:
        print(f'{ts} {INFO} {green}COMMAND mode{reset}: using your custom flags.')
    else:
        # Manual mode: show level + profile menus
        print(f'''
    {red}[{white}1{red}] {white}Weak       (char join)
    {red}[{white}2{red}] {white}Medium     (Base64)
    {red}[{white}3{red}] {white}Strong     (Base64 + variable)
    {red}[{white}4{red}] {white}Very Strong (GZip + Base64)
    {red}[{white}5{red}] {white}Extreme    (GZip + XOR + fragmentation + noise)
        ''')

        PROFILE_DESCS = {
            'safe':     'compat / safe',
            'light':    'light obfuscation',
            'balanced': 'balanced (default)',
            'heavy':    'heavy obfuscation',
            'stealth':  'stealth / evasion',
            'paranoid': 'maximum paranoia',
            'redteam':  'red-team ops',
            'blueteam': 'blue-team / audit',
            'size':     'minimize size',
            'dev':      'dev / debug',
        }
        print(f'    {red}Profiles:{reset}')
        for i, pname in enumerate(VALID_PROFILES, 1):
            desc = PROFILE_DESCS.get(pname, pname)
            print(f'    {red}[{white}{i:>2}{red}] {white}{pname:<10}{reset} ({desc})')
        print()

        try:
            level_input = input(f'{ts} {INPUT} Obfuscation level (1-5, default=3) -> {reset}').strip()
            obfuscation_force = int(level_input) if level_input else 3
        except (ValueError, KeyboardInterrupt):
            print(f'{ts} {ERROR} Invalid number.')
            return
        if obfuscation_force not in (1, 2, 3, 4, 5):
            print(f'{ts} {ERROR} Choose 1 to 5.')
            return

        try:
            profile_input = input(f'{ts} {INPUT} Profile (1-{len(VALID_PROFILES)} or name, default=3/balanced) -> {reset}').strip()
        except KeyboardInterrupt:
            print()
            return
        profile = 'balanced'
        if profile_input:
            if profile_input.isdigit():
                pidx = int(profile_input)
                if 1 <= pidx <= len(VALID_PROFILES):
                    profile = VALID_PROFILES[pidx - 1]
                else:
                    print(f'{ts} {INFO} Invalid number "{profile_input}", using "balanced".')
            elif profile_input.lower() in VALID_PROFILES:
                profile = profile_input.lower()
            else:
                print(f'{ts} {INFO} Unknown profile "{profile_input}", using "balanced".')

        use_ast = input(f'{ts} {INPUT} Use native AST when available? [Y/n] -> {reset}').strip().lower() != 'n'
        do_validate = input(f'{ts} {INPUT} Validate output after obfuscation? [y/N] -> {reset}').strip().lower() == 'y'

    # Pre-read files that live inside the output folder (they'll be deleted during cleanup).
    abs_folder = os.path.abspath(folder)
    file_contents = {}  # path -> bytes (cached for files inside ObfusPS/)
    for fp in valid_files:
        try:
            if os.path.abspath(fp).startswith(abs_folder + os.sep):
                with open(fp, 'rb') as f:
                    file_contents[fp] = f.read()
                print(f'{ts} {INFO} Cached: {white}{os.path.basename(fp)}{reset} (inside output folder)')
        except Exception as e:
            print(f'{ts} {ERROR} Cannot read {fp}: {e}')

    print(f'{ts} {WAIT} Removing previous folders..')
    try:
        shutil.rmtree(folder, ignore_errors=True)
    except Exception as e:
        print(f'{ts} {ERROR} Cannot remove {folder}: {e}')
        return
    print(f'{ts} {INFO} Previous folders removed.')
    try:
        print(f'{ts} {INFO} Creating folder: {white}{folder}{reset}')
        os.makedirs(script_folder, exist_ok=True)
        os.makedirs(output_folder_1, exist_ok=True)
    except OSError as e:
        print(f'{ts} {ERROR} Cannot create folders: {e}')
        return

    used_names = {}
    total_files = len(valid_files)
    for idx, file_ps1 in enumerate(valid_files, 1):
        base_name = os.path.basename(file_ps1)
        if base_name not in used_names:
            used_names[base_name] = 0
        else:
            used_names[base_name] += 1
        if used_names[base_name] == 0:
            file_name = base_name
        else:
            base, ext = os.path.splitext(base_name)
            file_name = f'{base}_{used_names[base_name]}{ext}'

        script_path = os.path.join(script_folder, file_name)
        output_path = os.path.join(output_folder_1, file_name)

        print(f'{ts} {INFO} File {idx}/{total_files}: {white}{file_name}{reset}')

        # Write backup copy (from cache if original was inside deleted folder, else copy)
        try:
            if file_ps1 in file_contents:
                with open(script_path, 'wb') as f:
                    f.write(file_contents[file_ps1])
            else:
                shutil.copy2(file_ps1, script_path)
            print(f'{ts} {INFO} Copied: {white}{file_name}{reset} -> {script_path}')
        except Exception as e:
            print(f'{ts} {ERROR} Cannot copy {file_name}: {e}')
            continue

        # Use the backup copy as input if original is gone (was inside ObfusPS/)
        input_path = file_ps1 if os.path.isfile(file_ps1) else script_path

        print(f'{ts} {WAIT} File {idx}/{total_files} - Obfuscating: {white}{file_name}{reset}..')
        try:
            kwargs = {
                'check': False,
                'capture_output': True,
                'encoding': 'utf-8',
                'errors': 'replace',
            }
            if sys.platform == 'win32':
                flags = getattr(subprocess, 'CREATE_NO_WINDOW', 0)
                if flags:
                    kwargs['creationflags'] = flags
            mode, value = runner

            if command_mode:
                # COMMAND mode: user-provided raw flags + auto -i/-o
                base_cmd = ['-i', input_path, '-o', output_path] + custom_flags
            elif use_auto:
                # AUTO mode: let the engine decide everything
                base_cmd = ['-i', input_path, '-o', output_path, '-auto', '-auto-retry', '-validate-stderr', 'ignore', '-validate-timeout', '60']
            else:
                # MANUAL mode
                base_cmd = ['-i', input_path, '-o', output_path, '-level', str(obfuscation_force), '-profile', profile, '-context-aware']
                if base_name.lower().endswith('.psm1'):
                    base_cmd.append('-module-aware')
                if use_ast:
                    base_cmd.append('-use-ast')
                if do_validate:
                    base_cmd.extend(['-validate', '-auto-retry', '-validate-stderr', 'ignore', '-validate-timeout', '60'])
                if obfuscation_force == 5:
                    base_cmd.extend(['-frag', 'profile=pro'])

            if mode == 'go_run':
                cmd = ['go', 'run', './cmd/obfusps'] + base_cmd
                kwargs['cwd'] = value
            else:
                cmd = [value] + base_cmd
            r = subprocess.run(cmd, **kwargs)

            # Print stderr output (contains analysis, metrics, etc.)
            if r.stderr:
                for line in r.stderr.strip().splitlines():
                    line = line.strip()
                    if line:
                        print(f'{ts} {INFO} {line}')

            if r.returncode != 0:
                err_msg = r.stderr.strip() if r.stderr else r.stdout.strip() if r.stdout else 'unknown error'
                print(f'{ts} {ERROR} obfusps failed (exit {r.returncode})')
                continue
        except Exception as e:
            print(f'{ts} {ERROR} Error: {e}')
            continue

        print(f'{ts} {ADD} Done: {white}{output_path}{reset}')

    print(f'{ts} {INFO} Obfuscation complete: {len(valid_files)} file(s) -> {white}{output_folder_1}{reset}')
    print(f'{ts} {INFO} Scripts compatible with PowerShell 5.1 & 7.x')
    print(f'{ts} {INFO} Architecture: Go engine; -use-ast for native AST when pwsh available.')


if __name__ == '__main__':
    try:
        while True:
            ObfusPS_Tool()
            try:
                input(f'{BEFORE + current_time_hour() + AFTER} {INPUT} Press Enter to continue.. ')
            except KeyboardInterrupt:
                print()
                break
    except KeyboardInterrupt:
        print()
