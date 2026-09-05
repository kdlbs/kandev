import type { QuickJSContext, QuickJSHandle } from "quickjs-emscripten";

export type OwnHandle = <T extends QuickJSHandle>(handle: T) => T;

export function setProp(
  context: QuickJSContext,
  target: QuickJSHandle,
  name: string | number,
  value: QuickJSHandle | string | boolean,
): void {
  const handle = toHandle(context, value);
  context.setProp(target, name, handle);
  if (typeof value === "string") handle.dispose();
}

function toHandle(context: QuickJSContext, value: QuickJSHandle | string | boolean): QuickJSHandle {
  if (typeof value === "string") return context.newString(value);
  if (typeof value === "boolean") return value ? context.true : context.false;
  return value;
}

export function addMethod(
  context: QuickJSContext,
  own: OwnHandle,
  target: QuickJSHandle,
  name: string,
  callback: (thisArg: QuickJSHandle, ...args: QuickJSHandle[]) => QuickJSHandle | void,
): void {
  const functionHandle = own(
    context.newFunction(name, function (this: QuickJSHandle, ...args) {
      return callback(this, ...args);
    }),
  );
  setProp(context, target, name, functionHandle);
}

export function addEphemeralMethod(
  context: QuickJSContext,
  target: QuickJSHandle,
  name: string,
  callback: () => QuickJSHandle,
): void {
  const functionHandle = context.newFunction(name, callback);
  setProp(context, target, name, functionHandle);
  functionHandle.dispose();
}

export function defineGetter(
  context: QuickJSContext,
  target: QuickJSHandle,
  name: string,
  getter: () => QuickJSHandle,
): void {
  context.defineProp(target, name, { configurable: true, enumerable: true, get: getter });
}

export function defineSetter(
  context: QuickJSContext,
  target: QuickJSHandle,
  name: string,
  setter: (value: QuickJSHandle) => void,
): void {
  context.defineProp(target, name, {
    configurable: true,
    enumerable: true,
    get: () => context.undefined,
    set: setter,
  });
}

export function defineValue(
  context: QuickJSContext,
  target: QuickJSHandle,
  name: string,
  value: string,
  options: { enumerable?: boolean; configurable?: boolean } = {},
): void {
  const handle = context.newString(value);
  context.defineProp(target, name, {
    value: handle,
    enumerable: options.enumerable ?? true,
    configurable: options.configurable ?? true,
  });
  handle.dispose();
}

export function toString(context: QuickJSContext, value: QuickJSHandle): string {
  return context.getString(value);
}

export function toNumber(context: QuickJSContext, value: QuickJSHandle): number {
  return context.getNumber(value);
}

export function toBoolean(context: QuickJSContext, value: QuickJSHandle): boolean {
  return Boolean(context.dump(value));
}

export function readHiddenString(
  context: QuickJSContext,
  value: QuickJSHandle,
  key: string,
): string | undefined {
  const property = context.getProp(value, key);
  try {
    return context.typeof(property) === "string" ? toString(context, property) : undefined;
  } finally {
    property.dispose();
  }
}

export function readStringArray(context: QuickJSContext, value: QuickJSHandle | undefined): string {
  if (!value || context.typeof(value) !== "object") return "";
  const length = context.getLength(value) ?? 0;
  const parts: string[] = [];
  for (let index = 0; index < length; index += 1) {
    const part = context.getProp(value, index);
    try {
      parts.push(toString(context, part));
    } finally {
      part.dispose();
    }
  }
  return parts.join("");
}

export function readTypeOption(context: QuickJSContext, value: QuickJSHandle): string {
  const property = context.getProp(value, "type");
  try {
    return context.typeof(property) === "string" ? toString(context, property) : "text/plain";
  } finally {
    property.dispose();
  }
}

export function readObjectProperties(
  context: QuickJSContext,
  object: QuickJSHandle,
): Array<[string, QuickJSHandle]> {
  const properties = context.unwrapResult(
    context.getOwnPropertyNames(object, {
      strings: true,
      numbersAsStrings: true,
      onlyEnumerable: true,
    }),
  );
  const values: Array<[string, QuickJSHandle]> = [];
  try {
    for (const property of properties) {
      const key = toString(context, property);
      property.dispose();
      values.push([key, context.getProp(object, key)]);
    }
  } finally {
    properties.dispose();
  }
  return values;
}

export function toDatasetKey(value: string): string {
  return value.replace(/-([a-z])/g, (_match, character: string) => character.toUpperCase());
}

export function fromDatasetKey(value: string): string {
  return value.replace(/[A-Z]/g, (character) => `-${character.toLowerCase()}`);
}

export function toStyleKey(value: string): string {
  return value.trim().replace(/-([a-z])/g, (_match, character: string) => character.toUpperCase());
}

export function fromStyleKey(value: string): string {
  return value.replace(/[A-Z]/g, (character) => `-${character.toLowerCase()}`);
}
