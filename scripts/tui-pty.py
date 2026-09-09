"""Linux acceptance using a real PTY, isolated daemon, and compiled terminal client."""
import errno, fcntl, json, os, pty, re, select, signal, struct, subprocess, sys, termios, time
exe, data, output = sys.argv[1:]
master, slave = pty.openpty()
fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 44, 140, 0, 0))
before = termios.tcgetattr(slave)
env = dict(os.environ, TERM='xterm-256color', COLORTERM='truecolor')
env.pop('NO_COLOR', None)
def terminal_session():
    os.setsid()
    fcntl.ioctl(slave, termios.TIOCSCTTY, 0)
process = subprocess.Popen([exe, 'tui', 'video-series', '--data-dir', data], stdin=slave, stdout=slave, stderr=slave, env=env, preexec_fn=terminal_session)
transcript = bytearray()
ansi = re.compile(r'\x1b\][^\x07]*(?:\x07|\x1b\\)|\x1b\[[0-?]*[ -/]*[@-~]|\x1b[()][0-2A-Z]|\x1b[=>]')
def read(duration=.15):
    until = time.monotonic()+duration
    while time.monotonic()<until:
        if select.select([master], [], [], max(0, until-time.monotonic()))[0]:
            try: transcript.extend(os.read(master, 65536))
            except OSError as e:
                if e.errno != errno.EIO: raise
                break
    return ansi.sub('', transcript.decode('utf-8', 'replace'))
def wait_text(text):
    until=time.monotonic()+10
    while time.monotonic()<until:
        if text in read(): return
        if process.poll() is not None: break
    raise AssertionError('Missing terminal output: '+text+'\n'+read())
def send(keys): os.write(master, keys.encode()); read(.25)
def state(): return json.loads(subprocess.check_output([exe,'state','--data-dir',data]))
def wait_state(predicate):
    until=time.monotonic()+10
    while time.monotonic()<until:
        if predicate(state()): return
        read(.1)
    raise AssertionError('Daemon did not record the explicit terminal action')
try:
    wait_text('workstreams')
    # Focus needs-you and reject the actual pending step completion proposal.
    send('\t\t\r'); wait_text('done when'); send('x')
    wait_state(lambda st: any(p['status']=='rejected' for p in st['proposals'].values()))
    # The decision is now the first need. Editing a note must not invoke shortcuts.
    send('\r'); wait_text('review note'); send('\tReviewed the proposed music change.'); send('\x1b'); send('a')
    wait_state(lambda st: len(st['progress_reviews'])==1)
    assert state()['tasks']['video-series']['task']['status']=='active'
    # Save root progress with a durable draft through the actual form.
    send('\tc'); wait_text('what changed'); send('Reviewed TUI checkpoint form.'); send('\x13')
    wait_state(lambda st: any(c['summary']=='Reviewed TUI checkpoint form.' for c in st['checkpoints'].values()))
    send('p'); wait_text('desktop preview'); send('\x1b')
    # Resize exercises compact mode in the same terminal and preserves selection.
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH',24,80,0,0)); os.kill(process.pid, signal.SIGWINCH)
    wait_text('WORKSTREAMS'); send('q'); process.wait(timeout=5)
    assert process.returncode==0
    after=termios.tcgetattr(slave)
    assert before==after, 'terminal settings were not restored'
    assert b'\x1b[?1049l' in transcript, 'alternate screen was not restored'
    print(json.dumps({'status':'passed','checks':['real PTY input','step completion rejection','decision note and acceptance','checkpoint form persistence','workspace preview','resize to compact','terminal restoration']}))
finally:
    if process.poll() is None: process.terminate(); process.wait(timeout=5)
    os.close(master);os.close(slave)
    with open(output,'wb') as f: f.write(transcript)
