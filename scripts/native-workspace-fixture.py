#!/usr/bin/env python3
"""Disposable native GTK view for explicit W05 acceptance; contains no user data."""
import gi, os, sys
gi.require_version('Gtk', '4.0')
from gi.repository import Gtk, GLib

app = Gtk.Application(application_id='org.heimdall.W05Fixture')
def activate(app):
    window = Gtk.ApplicationWindow(application=app, title='Heimdall workspace fixture')
    window.set_default_size(520, 240)
    label = Gtk.Label(label='Heimdall workspace operation check\n\nSynthetic view · no user data\n\nFocus and graceful closure use the shared action journal.')
    label.set_margin_start(24)
    label.set_margin_end(24)
    label.set_margin_top(24)
    label.set_margin_bottom(24)
    window.set_child(label)
    window.present()
    print(os.getpid(), flush=True)
    GLib.timeout_add_seconds(120, lambda: app.quit())
app.connect('activate', activate)
app.run([sys.argv[0]])
