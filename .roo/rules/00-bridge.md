# FoxTrack Bridge rules for Roo

## What this is
An open-source Go binary that runs on a user's own machine and reads local
printer telemetry (Bambu Lab MQTT, Klipper/Moonraker). It ships as a single
binary with an embedded web UI. Outside contributors read this code.

## Never touch
- go.mod, go.sum (adding or changing a dependency is not a Roo task)
- .github/, Dockerfile, release scripts
If a task seems to need these, STOP and say so instead of editing.

## Never run
Any git, go get, go mod, docker, or release command. The owner runs all commands.

## Extra care
Printer connection code, MQTT and Moonraker protocol handling, and the config
file format are high risk. A breaking config change costs every existing user
their saved printers. If a task touches any of these, make the smallest possible
change and say clearly what you changed.

## Style
Standard Go. Run formatting as gofmt would produce it.
Return errors, do not panic in library code.
Do not introduce new dependencies. Use the standard library.
Keep the embedded UI dependency-free: no new frameworks, no CDN scripts.

## Copy
Sentence case. No em dashes anywhere, in copy or code.

## Behaviour
Edit only the files listed in the task. Do not search the repo unless told to.
Do not refactor anything not asked for. Do not add comments explaining changes.
When done, list the files you changed and stop.
