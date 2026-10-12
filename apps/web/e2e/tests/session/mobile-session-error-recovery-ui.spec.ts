import { automaticRecoveryOwnerScenario } from "../../helpers/automatic-recovery-owner";
import { recoveryDraftScenario } from "../../helpers/session-error-recovery-ui";
import {
  sessionErrorDetailsScenario,
  uniformRecoveryCases,
} from "../../helpers/session-error-recovery-ui";
sessionErrorDetailsScenario();

uniformRecoveryCases();

recoveryDraftScenario();

automaticRecoveryOwnerScenario();
