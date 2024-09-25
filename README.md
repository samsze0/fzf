# Fzf, with websockets

A fork of fzf that supports websockets as a source of inter-process communication.

This fork is intended for those of you who are interested in building some kind of tools that revolves around fzf. Before the existence of this fork, there is no clean ways to subscribe to the "state" of fzf (e.g. the current query, the current selection, etc.) from another process. This fork aims to solve this problem by providing the options to spawn a websocket server that you can connect to.

There is currently two "implementations" of the websocket server. Either you can spawn a server, or you can spawn a client that acts like a server. You might wonder, why would you want to spawn a client that acts like a server? This is partcularly useful if you want to spawn a fzf instance that connects to a websocket server right away.

If you want to spawn a server, you can use the `--websocket-listen` option like so:
```zsh
FZF_API_KEY="key" fzf \
  --websocket-listen="localhost:12010" \
  --bind="start:websocket-broadcast@Hi from fzf@"
```

If you want to spawn a client that acts like a server, you can use the `--websocket-listen-to` option like so:
```
FZF_API_KEY="key" fzf \
  --websocket-listen-to="ws://localhost:12010" \
  --bind="start:websocket-broadcast@Hi from fzf@"
```

---

<img src="https://raw.githubusercontent.com/junegunn/i/master/fzf.png" height="170" alt="fzf - a command-line fuzzy finder"> [![github-actions](https://github.com/junegunn/fzf/workflows/Test%20fzf%20on%20Linux/badge.svg)](https://github.com/junegunn/fzf/actions)
===

fzf is a general-purpose command-line fuzzy finder.

<img src="https://raw.githubusercontent.com/junegunn/i/master/fzf-preview.png" width=640>

It's an interactive filter program for any kind of list; files, command
history, processes, hostnames, bookmarks, git commits, etc. It implements
a "fuzzy" matching algorithm, so you can quickly type in patterns with omitted
characters and still get the results you want.

[License](LICENSE)
------------------

The MIT License (MIT)

Copyright (c) 2013-2024 Junegunn Choi
