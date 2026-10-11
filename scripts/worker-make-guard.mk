# Included before build prerequisites so dispatch precedes all heavy work.
WORKER_CHECK_GOALS := $(strip $(if $(MAKECMDGOALS),$(MAKECMDGOALS),$(WORKER_CHECK_DEFAULT)))
ifeq ($(FULL_WORKER_CHECK_MODE),isolated)
ifneq ($(FULL_WORKER_CHECK_INSIDE),1)
ifneq ($(filter build% test% lint% all,$(WORKER_CHECK_GOALS)),)
WORKER_CHECK_DISPATCH := 1
WORKER_CHECK_KIND := $(if $(filter lint%,$(WORKER_CHECK_GOALS)),lint,$(if $(filter test-e2e%,$(WORKER_CHECK_GOALS)),browser,$(if $(filter build% all,$(WORKER_CHECK_GOALS)),build,test)))
worker_quote = '$(subst ','"'"',$(1))'
.DEFAULT_GOAL := worker-check-dispatch
.PHONY: worker-check-dispatch $(WORKER_CHECK_GOALS)
$(WORKER_CHECK_GOALS): worker-check-dispatch
	@:
worker-check-dispatch:
	@$(call worker_quote,$(WORKER_CHECK_ROOT)/scripts/worker-check) --kind $(WORKER_CHECK_KIND) -- $(MAKE) --no-print-directory $(foreach goal,$(WORKER_CHECK_GOALS),$(call worker_quote,$(goal)))
endif
endif
endif
