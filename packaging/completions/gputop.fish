# fish completion for gputop
#
# Installed as /usr/share/fish/vendor_completions.d/gputop.fish. The option
# list is the one cmd/gputop/main.go registers; scripts/check_packaging.sh
# fails when the two drift apart.

# gputop takes options only, so plain-argument file completion is noise.
complete -c gputop -f

complete -c gputop -s c -l config -r -F -d 'configuration file to load'
complete -c gputop -s i -l interval -r -x -d 'sampling interval in seconds'
complete -c gputop -l drm-root -r -F -d 'sysfs DRM class directory'
complete -c gputop -l proc-root -r -F -d 'procfs mount point'
complete -c gputop -l dump -d 'write one JSON snapshot to stdout and exit'
complete -c gputop -l pretty -d 'indent --dump output for human reading'
complete -c gputop -l devices -d 'list discovered GPUs as JSON and exit'
complete -c gputop -l kind -x -a 'auto igpu dgpu' -d 'force device classification'
complete -c gputop -l theme -x -a 'default dracula gruvbox monochrome radeon' -d 'colour theme'
complete -c gputop -l no-color -d 'never emit colour'
complete -c gputop -l no-processes -d 'skip the /proc scan'
complete -c gputop -l blocks -d 'force the per-block panel on'
complete -c gputop -l no-blocks -d 'never start radeontop'
complete -c gputop -l log -r -F -d 'record the session to a file'
complete -c gputop -l check -d 'report what this machine can provide'
complete -c gputop -l json -d 'emit --check as JSON'
complete -c gputop -l version -d 'print the version and exit'
