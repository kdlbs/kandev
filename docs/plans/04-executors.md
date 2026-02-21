also i think you're miss understanding a bit the sprites.dev executor:
  - the user should not be able to change its name, an Executor can't be changed by the user, he can only change the profile he created
  - connection and "Configure a SPRITES_API_TOKEN environment variable in the executor profile, referencing a secret with your Sprites.dev API token." is wrong, so the env variable is wrong, the secrets are now key value only, and
  the connection and running sprites that are in the page settings/executor/exec-sprites are wrong because they should belong to a sprites profile, not the executor

  the executor page should only have the option for the user to create a new profile and list the existing profiles
  each executor profile is specific to that executor, it can't be "generic" or shared.

  when creating an executor profile, lets say sprites.dev, the should needs to specify the api key (we should display a selector for him to choose from the secrets)

  e.g for the sprites.dev profile the user should provide:
  - api key (select from secrets)
  - test connection check
  - env variables list (that the user can specity or use values from secrets), let the user know these variables will be inject for the runtime and setup script
  - prepare env script
  - cleanup env script
  actually i think its a good idea for the prepare env to be a bash script, with the steps we need (e.g install deps, git clone, setup permissions, etc) but using placeholders (templating) that we'll parse and run
  so the user can see clearly what will run and change it, our default scripts should have good comments to guide the user, and on the interface use the monaco editor with syntax highlighting/good editing capabilities
  (when opening a sprits.dev profile, we should list the running sprites and existing network policies and let the user stop/remove them)


  so this prepare env script would have several placeholders like (examples, you need to adapt):
  #!/bin/bash
  # install deps
  apt-install node npm ca-certificates

  # setup git
  git clone {repository.ssh_url}
  cd {repository.name}
  # run repository setup script if configured
  {repository.setup.script}

  {kandev.agentctl.install} # pulls from github or copies on make dev?
  {kandev.agentctl.start} # runs our agentctl process with our flags

  each executor should have a default prepare script, that is filled when the user creates a new profile, he then can change it

  lets go for each executor:
  - Local
  we should create a Local profile by default, its profile should only have a prepare setup script that runs git pull / fetch?
  (we should provide info to the user to let him know this will run on the chosen repo)

  - Worktree
  we should create a Worktree profile by default, its profile should only have a prepare setup script that runs git pull / fetch?

  - Docker
  we should create a "Local Docker" profile by default, its profile should have:
  - docker host value
  - type: local, remote (local for now)
  - base image
    - build custom image + editor (have a default dockerfile contents with at least node and git, and the user can change it)
    - build args
  - volumes
    should be a list, with our added volumes (like the repo, the agentctl binary, etc disabled) and the user can add more if he wants
  - env variables

  (when opening a docker profile, we should list the running containers and let the user stop/remove them)

  so we can use this docker executor as well to let the user a remote docker profile, he would need to provide a host and ssh config? or only a remote docker host? but what about authentication?
  anything the local and remote docker profiles should be here

  so we can remove the concept of an environment entirely and the dedicated settings page for it, each executor profile will have its "environment"
  in the create task dialog, we list the existing executor profiles to run (not the executors type name, only the existing profiles)
