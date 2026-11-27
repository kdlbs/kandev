#include <X11/Xatom.h>
#include <X11/Xlib.h>
#include <X11/extensions/XTest.h>
#include <X11/keysym.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static int window_pid(Display *display, Window window, unsigned long *pid_out) {
  Atom property = XInternAtom(display, "_NET_WM_PID", True);
  if (property == None) return 0;

  Atom actual_type;
  int actual_format;
  unsigned long item_count;
  unsigned long bytes_after;
  unsigned char *data = NULL;
  int result = XGetWindowProperty(display, window, property, 0, 1, False, XA_CARDINAL,
                                  &actual_type, &actual_format, &item_count,
                                  &bytes_after, &data);
  if (result != Success || actual_type != XA_CARDINAL || actual_format != 32 ||
      item_count != 1 || data == NULL) {
    if (data != NULL) XFree(data);
    return 0;
  }

  *pid_out = *(unsigned long *)data;
  XFree(data);
  return 1;
}

static int is_kandev_window(Display *display, Window window) {
  XWindowAttributes attributes;
  if (!XGetWindowAttributes(display, window, &attributes) ||
      attributes.map_state != IsViewable) {
    return 0;
  }

  char *name = NULL;
  int matches = XFetchName(display, window, &name) && name != NULL &&
                strcmp(name, "Kandev") == 0;
  if (name != NULL) XFree(name);
  return matches;
}

static Window find_window(Display *display, Window parent, unsigned long expected_pid) {
  Window root;
  Window parent_return;
  Window *children = NULL;
  unsigned int child_count = 0;
  if (!XQueryTree(display, parent, &root, &parent_return, &children, &child_count)) {
    return None;
  }

  Window found = None;
  for (unsigned int offset = child_count; offset > 0; offset--) {
    Window child = children[offset - 1];
    if (is_kandev_window(display, child)) {
      unsigned long pid = 0;
      if (expected_pid == 0 ||
          (window_pid(display, child, &pid) && pid == expected_pid)) {
        found = child;
        break;
      }
    }
    found = find_window(display, child, expected_pid);
    if (found != None) break;
  }
  if (children != NULL) XFree(children);
  return found;
}

static int focus_window(Display *display, Window window) {
  if (window == None) return 0;
  XMapRaised(display, window);
  XRaiseWindow(display, window);
  XSetInputFocus(display, window, RevertToParent, CurrentTime);
  XSync(display, False);
  usleep(100000);
  return 1;
}

int main(int argc, char **argv) {
  if (argc < 2) {
    fprintf(stderr, "usage: x11-window-input find [pid] | activate <pid> | quit <pid>\n");
    return 2;
  }

  Display *display = XOpenDisplay(NULL);
  if (display == NULL) {
    fprintf(stderr, "could not open the X11 display\n");
    return 2;
  }

  unsigned long expected_pid = argc >= 3 ? strtoul(argv[2], NULL, 10) : 0;
  Window window = find_window(display, DefaultRootWindow(display), expected_pid);
  if (window == None) {
    XCloseDisplay(display);
    return 1;
  }

  if (strcmp(argv[1], "find") == 0) {
    printf("%lu\n", window);
    XCloseDisplay(display);
    return 0;
  }
  if (!focus_window(display, window)) {
    XCloseDisplay(display);
    return 1;
  }

  if (strcmp(argv[1], "activate") == 0) {
    XWindowAttributes attributes;
    if (!XGetWindowAttributes(display, window, &attributes)) {
      XCloseDisplay(display);
      return 1;
    }
    int local_x = attributes.width / 40;
    int local_y = attributes.height / 8;
    int root_x;
    int root_y;
    Window child;
    XTranslateCoordinates(display, window, DefaultRootWindow(display), local_x,
                          local_y, &root_x, &root_y, &child);
    XTestFakeMotionEvent(display, DefaultScreen(display), root_x, root_y, CurrentTime);
    XTestFakeButtonEvent(display, 1, True, CurrentTime);
    XTestFakeButtonEvent(display, 1, False, CurrentTime);
    XSync(display, False);
    usleep(100000);
    KeyCode tab = XKeysymToKeycode(display, XK_Tab);
    KeyCode enter = XKeysymToKeycode(display, XK_Return);
    XTestFakeKeyEvent(display, tab, True, CurrentTime);
    XTestFakeKeyEvent(display, tab, False, CurrentTime);
    XTestFakeKeyEvent(display, tab, True, CurrentTime);
    XTestFakeKeyEvent(display, tab, False, CurrentTime);
    XSync(display, False);
    usleep(100000);
    XTestFakeKeyEvent(display, enter, True, CurrentTime);
    XTestFakeKeyEvent(display, enter, False, CurrentTime);
    XFlush(display);
    XCloseDisplay(display);
    return 0;
  }

  if (strcmp(argv[1], "quit") == 0) {
    KeyCode control = XKeysymToKeycode(display, XK_Control_L);
    KeyCode q = XKeysymToKeycode(display, XK_q);
    XTestFakeKeyEvent(display, control, True, CurrentTime);
    XTestFakeKeyEvent(display, q, True, CurrentTime);
    XTestFakeKeyEvent(display, q, False, CurrentTime);
    XTestFakeKeyEvent(display, control, False, CurrentTime);
    XFlush(display);
    XCloseDisplay(display);
    return 0;
  }

  fprintf(stderr, "unknown command: %s\n", argv[1]);
  XCloseDisplay(display);
  return 2;
}
