type PersistentMotionVisibilityOptions = {
  ownerVisible?: boolean;
};

type MotionElement = HTMLElement | SVGElement;

type PersistentMotionRegistration = {
  setAnimation: (animation: Animation | null) => void;
  unregister: () => void;
};

type PersistentMotionVisibilityController = {
  register: (element: MotionElement, animation?: Animation | null) => PersistentMotionRegistration;
  setOwnerVisible: (visible: boolean) => void;
  isVisible: () => boolean;
  dispose: () => void;
};

type Registration = {
  element: MotionElement;
  animation: Animation | null;
  previousPlayState: string;
  cssPaused: boolean;
  resumeAnimation: boolean;
  active: boolean;
};

type DocumentVisibilitySubscription = {
  subscribers: Set<() => void>;
  listener: () => void;
};

const documentVisibilitySubscriptions = new WeakMap<Document, DocumentVisibilitySubscription>();

function subscribeToDocumentVisibility(doc: Document, listener: () => void): () => void {
  let subscription = documentVisibilitySubscriptions.get(doc);
  if (!subscription) {
    const subscribers = new Set<() => void>();
    const onVisibilityChange = () => {
      for (const subscriber of subscribers) subscriber();
    };
    subscription = { subscribers, listener: onVisibilityChange };
    documentVisibilitySubscriptions.set(doc, subscription);
    doc.addEventListener("visibilitychange", onVisibilityChange);
  }

  subscription.subscribers.add(listener);
  let subscribed = true;
  return () => {
    if (!subscribed) return;
    subscribed = false;
    subscription?.subscribers.delete(listener);
    if (subscription?.subscribers.size !== 0) return;
    doc.removeEventListener("visibilitychange", subscription.listener);
    documentVisibilitySubscriptions.delete(doc);
  };
}

function playOwnedAnimation(animation: Animation): void {
  try {
    animation.play();
  } catch {
    // A handle can be canceled by its owner between the visibility event and
    // this callback. Cleanup remains the owner's responsibility.
  }
}

function pauseOwnedAnimation(animation: Animation): void {
  try {
    animation.pause();
  } catch {
    // A canceled handle no longer needs lifecycle control.
  }
}

export function createPersistentMotionVisibility(
  target: Element,
  options: PersistentMotionVisibilityOptions = {},
): PersistentMotionVisibilityController {
  const doc = target.ownerDocument;
  let ownerVisible = options.ownerVisible ?? true;
  let intersectionVisible = true;
  let currentVisible = doc.visibilityState !== "hidden" && ownerVisible;
  let disposed = false;
  const registrations = new Set<Registration>();
  const registeredElements = new Set<MotionElement>();
  const notify = () => {
    if (disposed) return;
    const nextVisible = doc.visibilityState !== "hidden" && ownerVisible && intersectionVisible;
    if (nextVisible === currentVisible) return;
    currentVisible = nextVisible;
    for (const registration of registrations) applyVisibility(registration, currentVisible);
  };

  const unsubscribeDocument = subscribeToDocumentVisibility(doc, notify);
  let observer: IntersectionObserver | null = null;
  const updateObservation = () => {
    if (!ownerVisible) {
      observer?.disconnect();
      observer = null;
      return;
    }
    if (observer || typeof IntersectionObserver === "undefined") return;
    const next = new IntersectionObserver((entries) => {
      if (observer !== next) return;
      const entry = entries[entries.length - 1];
      if (!entry) return;
      // Threshold zero includes an intersecting target at the viewport edge.
      intersectionVisible = entry.isIntersecting;
      notify();
    });
    observer = next;
    observer.observe(target);
  };
  updateObservation();

  const unregister = (registration: Registration) => {
    if (!registration.active) return;
    registration.active = false;
    registrations.delete(registration);
    registeredElements.delete(registration.element);
    if (registration.cssPaused) {
      registration.element.style.animationPlayState = registration.previousPlayState;
      registration.cssPaused = false;
    }
    registration.animation = null;
    registration.resumeAnimation = false;
  };

  return {
    register(element, animation = null) {
      if (registeredElements.has(element)) {
        throw new Error("An element can only be registered once per visibility controller");
      }
      registeredElements.add(element);
      const registration: Registration = {
        element,
        animation,
        previousPlayState: element.style.animationPlayState,
        cssPaused: false,
        resumeAnimation: false,
        active: true,
      };
      registrations.add(registration);
      applyVisibility(registration, currentVisible);

      let active = true;
      return {
        setAnimation(nextAnimation) {
          if (!active) return;
          registration.animation = nextAnimation;
          registration.resumeAnimation = false;
          applyVisibility(registration, currentVisible);
        },
        unregister() {
          if (!active) return;
          active = false;
          unregister(registration);
        },
      };
    },
    setOwnerVisible(visible) {
      if (ownerVisible === visible) return;
      ownerVisible = visible;
      updateObservation();
      notify();
    },
    isVisible() {
      return currentVisible;
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      observer?.disconnect();
      observer = null;
      unsubscribeDocument();
      for (const registration of [...registrations]) unregister(registration);
    },
  };
}

function applyVisibility(registration: Registration, visible: boolean): void {
  if (!registration.active) return;
  if (!visible) {
    // Only resume an effect that this controller paused; caller-paused and
    // reduced-motion effects remain paused when the target becomes visible.
    if (registration.animation && registration.animation.playState === "running") {
      registration.resumeAnimation = true;
      pauseOwnedAnimation(registration.animation);
    }
    if (!registration.cssPaused) {
      registration.element.style.animationPlayState = "paused";
      registration.cssPaused = true;
    }
    return;
  }

  if (registration.cssPaused) {
    registration.element.style.animationPlayState = registration.previousPlayState;
    registration.cssPaused = false;
  }
  if (registration.resumeAnimation && registration.animation) {
    playOwnedAnimation(registration.animation);
  }
  registration.resumeAnimation = false;
}

export type {
  PersistentMotionRegistration,
  PersistentMotionVisibilityController,
  PersistentMotionVisibilityOptions,
};
