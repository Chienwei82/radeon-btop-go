#compdef gputop
#
# zsh completion for gputop.
#
# Installed as /usr/share/zsh/site-functions/_gputop, so compinit picks it up
# from fpath. The option list is the one cmd/gputop/main.go registers;
# scripts/check_packaging.sh fails when the two drift apart.

_gputop() {
    _arguments -s -S \
        '(-c --config)'{-c,--config}'[configuration file]:config file:_files' \
        '(-i --interval)'{-i,--interval}'[sampling interval in seconds]:seconds:' \
        '--drm-root[sysfs DRM class directory]:directory:_directories' \
        '--proc-root[procfs mount point]:directory:_directories' \
        '--dump[write one JSON snapshot to stdout and exit]' \
        '--pretty[indent --dump output for human reading]' \
        '--devices[list discovered GPUs as JSON and exit]' \
        '--kind[force every device to be igpu or dgpu]:kind:(auto igpu dgpu)' \
        '--theme[colour theme]:theme:(default dracula gruvbox monochrome radeon)' \
        '--no-color[never emit colour]' \
        '--no-processes[skip the /proc scan]' \
        '(--no-blocks)--blocks[force the per-block panel on]' \
        '(--blocks)--no-blocks[never start radeontop]' \
        '--log[record the session to a file]:log file:_files' \
        '--check[report what this machine can provide and why anything is missing]' \
        '--json[emit --check as JSON]' \
        '--version[print the version and exit]'
}

_gputop "$@"
