#!/usr/bin/env python3
"""Synthetic read-only compositor IPC for the compiled W02 acceptance test."""
import json, os, socket, sys, threading
root, state, audit = sys.argv[1:]
connections = []
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
                def request(conn):
                    with conn:
                        cmd = conn.recv(100).decode()
                        if not cmd:
                            return
                        assert cmd in ('j/version', 'j/clients', 'j/workspaces', 'j/monitors'), cmd
                        with open(audit, 'a') as f:
                            f.write(cmd+'\n')
                        with open(state) as f:
                            values = json.load(f)
                        conn.sendall(json.dumps(values[cmd[2:]]).encode())
                threading.Thread(target=request, args=(conn,), daemon=True).start()
    threading.Thread(target=loop, daemon=True).start()
serve('.socket2.sock')
serve('.socket.sock')
print('ready', flush=True)
for line in sys.stdin:
    for conn in connections[:]:
        try:
            if line.strip() == 'gap':
                conn.close()
                connections.remove(conn)
            else:
                conn.sendall(line.encode())
        except OSError:
            connections.remove(conn)
