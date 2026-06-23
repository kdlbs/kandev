import "./styles.css";

type DesktopStatus = {
  title: string;
  detail: string;
  failed?: boolean;
};

declare global {
  interface Window {
    __KANDEV_DESKTOP_SET_STATUS?: (status: DesktopStatus) => void;
  }
}

const title = document.querySelector<HTMLHeadingElement>("#status-title");
const detail = document.querySelector<HTMLParagraphElement>("#status-detail");
const shell = document.querySelector<HTMLElement>(".startup-shell");

window.__KANDEV_DESKTOP_SET_STATUS = (status) => {
  if (title) {
    title.textContent = status.title;
  }
  if (detail) {
    detail.textContent = status.detail;
  }
  shell?.classList.toggle("is-failed", status.failed === true);
};
