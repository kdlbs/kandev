import "./styles.css";

type DesktopStatus = {
  title: string;
  detail: string;
  failed?: boolean;
};

declare global {
  interface Window {
    __KANDEV_DESKTOP_SET_STATUS?: (status: DesktopStatus) => void;
    __KANDEV_DESKTOP_PENDING_STATUS?: DesktopStatus;
  }
}

const title = document.querySelector<HTMLHeadingElement>("#status-title");
const detail = document.querySelector<HTMLParagraphElement>("#status-detail");
const shell = document.querySelector<HTMLElement>(".startup-shell");

const applyStatus = (status: DesktopStatus) => {
  if (title) {
    title.textContent = status.title;
  }
  if (detail) {
    detail.textContent = status.detail;
  }
  shell?.setAttribute("aria-busy", status.failed === true ? "false" : "true");
  shell?.classList.toggle("is-failed", status.failed === true);
};

window.__KANDEV_DESKTOP_SET_STATUS = applyStatus;
if (window.__KANDEV_DESKTOP_PENDING_STATUS) {
  applyStatus(window.__KANDEV_DESKTOP_PENDING_STATUS);
}
