#!/usr/bin/env python3
"""Synthetic native operation IPC. Dispatcher vocabulary is deliberately fixed."""
import json, os, re, socket, sys, threading, time
root, state, audit = sys.argv[1:]
lock = threading.Lock()
connections = []

def handle(conn):
    with conn:
        command = conn.recv(4096).decode()
        if not command:
            return
        with lock:
            with open(state) as f:
                values = json.load(f)
            if command.startswith('j/'):
                key = command[2:]
                assert key in ('version', 'clients', 'workspaces', 'monitors', 'status', 'activewindow')
                reply = json.dumps(values[key]).encode()
                delay = 0
            else:
                match = re.fullmatch(r'dispatch (focuswindow|closewindow) stableid:([0-9a-f]+)', command)
                assert match, command
                kind, stable_id = match.groups()
                window = next((w for w in values['clients'] if w['stableId'] == stable_id), None)
                assert window is not None
                if kind == 'focuswindow':
                    values['activewindow'] = window
                elif values.get('close_mode') != 'remain':
                    values['clients'] = [w for w in values['clients'] if w['stableId'] != stable_id]
                    values['activewindow'] = {}
                with open(state+'.next', 'w') as f:
                    json.dump(values, f)
                os.replace(state+'.next', state)
                reply = b'ok'
                delay = 3 if values.get('close_mode') == 'pause_close' and kind == 'closewindow' else 0
            with open(audit, 'a') as f:
                f.write(command+'\n')
        if delay:
            time.sleep(delay)
        try:
            conn.sendall(reply)
        except (BrokenPipeError, ConnectionResetError):
            pass

def serve(name):
    listener = socket.socket(socket.AF_UNIX)
    listener.bind(os.path.join(root, name))
    listener.listen()
    def loop():
        while True:
            conn, _ = listener.accept()
            if name == '.socket2.sock':
                connections.append(conn)
            else:
                threading.Thread(target=handle, args=(conn,), daemon=True).start()
    threading.Thread(target=loop, daemon=True).start()

serve('.socket2.sock')
serve('.socket.sock')
print('ready', flush=True)
for line in sys.stdin:
    for conn in connections[:]:
        try:
            conn.sendall(line.encode())
        except OSError:
            connections.remove(conn)
