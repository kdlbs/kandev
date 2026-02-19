import React, { useMemo } from "react";
import setiIconTheme from "@/lib/assets/file-icons/seti-icon-theme.json";

interface SetiIconDefinition {
  fontCharacter?: string;
  fontColor?: string;
}

interface FileIconGlyph {
  character: string;
  color?: string;
}

const setiIconDefinitions: Record<string, SetiIconDefinition> = setiIconTheme.iconDefinitions;
const setiDefaultIconId = setiIconTheme.file;
const setiFileNames = setiIconTheme.fileNames as Record<string, string>;
const setiFileExtensions = setiIconTheme.fileExtensions as Record<string, string>;
const setiLanguageIds = setiIconTheme.languageIds as Record<string, string>;

const setiDefaultIconDefinition: SetiIconDefinition = setiIconDefinitions[setiDefaultIconId] ?? {
  fontCharacter: "\\E023",
};

const decodeFontCharacter = (encoded?: string): string => {
  if (!encoded) return "";
  if (!encoded.startsWith("\\")) return encoded;

  const hex = encoded.slice(1);
  const codePoint = Number.parseInt(hex, 16);
  if (Number.isNaN(codePoint)) return "";

  return String.fromCodePoint(codePoint);
};

const collectExtensionCandidates = (fileName: string): string[] => {
  const parts = fileName.split(".");
  if (parts.length <= 1) return [];

  const candidates: string[] = [];
  for (let i = 1; i < parts.length; i++) {
    const candidate = parts.slice(i).join(".");
    if (candidate) {
      candidates.push(candidate);
    }
  }

  return candidates;
};

/** Look up a key in a map, falling back to its lowercase variant. */
const lookupWithLowerFallback = (map: Record<string, string>, key: string): string | undefined => {
  const direct = map[key];
  if (direct) return direct;
  const lower = key.toLowerCase();
  return lower !== key ? map[lower] : undefined;
};

/** Search candidates against a map with case-insensitive fallback. */
const findInCandidates = (
  map: Record<string, string>,
  candidates: string[],
): string | undefined => {
  for (const candidate of candidates) {
    const match = lookupWithLowerFallback(map, candidate);
    if (match) return match;
  }
  return undefined;
};

const resolveSetiIconId = (fileName: string): string | undefined => {
  // Direct file name match
  const nameMatch = lookupWithLowerFallback(setiFileNames, fileName);
  if (nameMatch) return nameMatch;

  // Dotfile name without leading dot
  if (fileName.startsWith(".") && fileName.length > 1) {
    const withoutDotMatch = setiFileNames[fileName.slice(1)];
    if (withoutDotMatch) return withoutDotMatch;
  }

  const extensionCandidates = collectExtensionCandidates(fileName);

  // File extension match
  const extMatch = findInCandidates(setiFileExtensions, extensionCandidates);
  if (extMatch) return extMatch;

  // Language ID match
  const lowerName = fileName.toLowerCase();
  const langMatch = setiLanguageIds[lowerName];
  if (langMatch) return langMatch;

  const langCandidateMatch = findInCandidates(setiLanguageIds, extensionCandidates);
  if (langCandidateMatch) return langCandidateMatch;

  // Dotfile language ID
  if (fileName.startsWith(".") && fileName.length > 1) {
    return setiLanguageIds[fileName.slice(1).toLowerCase()];
  }

  return undefined;
};

const getSetiIconForFile = (fileName: string): FileIconGlyph => {
  if (!fileName) {
    return {
      character: decodeFontCharacter(setiDefaultIconDefinition.fontCharacter) || " ",
      color: setiDefaultIconDefinition.fontColor,
    };
  }

  const iconId = resolveSetiIconId(fileName);
  const iconDefinition = iconId ? setiIconDefinitions[iconId] : undefined;

  return {
    character:
      decodeFontCharacter(iconDefinition?.fontCharacter) ||
      decodeFontCharacter(setiDefaultIconDefinition.fontCharacter) ||
      " ",
    color: iconDefinition?.fontColor ?? setiDefaultIconDefinition.fontColor,
  };
};

const BASE_ICON_STYLE: React.CSSProperties = {
  fontFamily:
    '"Seti", "Geist Mono", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
  fontSize: 14,
  lineHeight: 1,
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "1rem",
  height: "1rem",
  userSelect: "none",
  fontStyle: "normal",
  fontWeight: "normal",
  letterSpacing: "normal",
};

export interface FileIconProps {
  fileName?: string | null;
  filePath?: string | null;
  className?: string;
  style?: React.CSSProperties;
}

export const FileIcon: React.FC<FileIconProps> = ({ fileName, filePath, className, style }) => {
  const targetName = fileName ?? (filePath ? (filePath.split("/").pop() ?? "") : "");

  const icon = useMemo(() => getSetiIconForFile(targetName ?? ""), [targetName]);

  if (!icon.character.trim()) {
    return null;
  }

  return (
    <span
      aria-hidden="true"
      className={className ?? undefined}
      style={{ ...BASE_ICON_STYLE, color: icon.color, ...style }}
    >
      {icon.character}
    </span>
  );
};
