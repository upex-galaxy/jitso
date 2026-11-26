# A

<!-- BEGIN JITSO block=2 added=1: managed by "jitso setup instructions"; edits inside this block are replaced -->
## JITSO

In a project that has `jitso/jitso.yaml`:

- Start worker sessions only with `jitso launch`. The guard denies the other ways of starting an agent.
- A launch takes a build. When no saved one fits (`jitso build list`), compose one just in time with `jitso build draft -`; `jitso arsenal` lists what it may name. Saving it with `jitso build save` is optional.
- Load the `jitso` skill before you launch.
<!-- END JITSO block=2 -->

# After
