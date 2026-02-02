import * as React from 'react';

const MOBILE_BREAKPOINT = 640;
const TABLET_BREAKPOINT = 1024;

export type Breakpoint = 'mobile' | 'tablet' | 'desktop';

export type ResponsiveBreakpoint = {
  breakpoint: Breakpoint;
  isMobile: boolean;
  isTablet: boolean;
  isDesktop: boolean;
};

function getBreakpoint(width: number): Breakpoint {
  if (width < MOBILE_BREAKPOINT) return 'mobile';
  if (width < TABLET_BREAKPOINT) return 'tablet';
  return 'desktop';
}

export function useResponsiveBreakpoint(): ResponsiveBreakpoint {
  const [breakpoint, setBreakpoint] = React.useState<Breakpoint>('desktop');

  React.useEffect(() => {
    const updateBreakpoint = () => {
      setBreakpoint(getBreakpoint(window.innerWidth));
    };

    // Set initial value
    updateBreakpoint();

    // Listen for resize events
    const mqlMobile = window.matchMedia(`(max-width: ${MOBILE_BREAKPOINT - 1}px)`);
    const mqlTablet = window.matchMedia(
      `(min-width: ${MOBILE_BREAKPOINT}px) and (max-width: ${TABLET_BREAKPOINT - 1}px)`
    );

    mqlMobile.addEventListener('change', updateBreakpoint);
    mqlTablet.addEventListener('change', updateBreakpoint);

    return () => {
      mqlMobile.removeEventListener('change', updateBreakpoint);
      mqlTablet.removeEventListener('change', updateBreakpoint);
    };
  }, []);

  return {
    breakpoint,
    isMobile: breakpoint === 'mobile',
    isTablet: breakpoint === 'tablet',
    isDesktop: breakpoint === 'desktop',
  };
}
