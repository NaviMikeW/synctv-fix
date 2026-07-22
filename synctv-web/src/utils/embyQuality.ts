export interface EmbyQualityOption {
  name: string;
  bitrate: number;
  url: string;
}

const QUALITY_PRESETS = [
  { name: "超清 16 Mbps", bitrate: 16_000_000 },
  { name: "高清 8 Mbps", bitrate: 8_000_000 },
  { name: "标清 4 Mbps", bitrate: 4_000_000 },
  { name: "流畅 2 Mbps", bitrate: 2_000_000 }
];

const DEVICE_STORAGE_KEY = "synctv-emby-device-id";

function randomID(): string {
  try {
    if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
      return crypto.randomUUID().replaceAll("-", "");
    }
    if (typeof crypto !== "undefined" && typeof crypto.getRandomValues === "function") {
      const bytes = new Uint8Array(16);
      crypto.getRandomValues(bytes);
      return Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join("");
    }
  } catch {
    // Fall through to the non-cryptographic compatibility fallback.
  }
  return `${Date.now().toString(16)}${Math.random().toString(16).slice(2)}`
    .padEnd(32, "0")
    .slice(0, 32);
}

function readOrCreate(storage: Storage | undefined, key: string): string {
  if (!storage) return randomID();
  try {
    const value = storage.getItem(key);
    if (value) return value;
    const created = randomID();
    storage.setItem(key, created);
    return created;
  } catch {
    return randomID();
  }
}

function browserStorage(type: "local" | "session"): Storage | undefined {
  if (typeof window === "undefined") return undefined;
  try {
    return type === "local" ? window.localStorage : window.sessionStorage;
  } catch {
    return undefined;
  }
}

function findSearchParam(url: URL, expectedName: string): [string, string] | undefined {
  const lowerName = expectedName.toLowerCase();
  for (const [name, value] of url.searchParams.entries()) {
    if (name.toLowerCase() === lowerName) return [name, value];
  }
  return undefined;
}

function getSearchParam(url: URL, name: string): string | undefined {
  return findSearchParam(url, name)?.[1];
}

function setSearchParam(url: URL, name: string, value: string): void {
  const existing = findSearchParam(url, name);
  url.searchParams.set(existing?.[0] || name, value);
}

function isConfigurableEmbyPlaylist(url: URL): boolean {
  const path = decodeURIComponent(url.pathname).toLowerCase();
  const hasVideoBitrate = getSearchParam(url, "VideoBitrate") !== undefined;

  // Emby normally returns /videos/<id>/master.m3u8. Depending on the configured
  // host and SyncTV proxy path it may also appear as /emby/videos/... or inside
  // a longer proxy path. The bitrate parameter is the stable identifying part.
  return hasVideoBitrate && path.includes("master.m3u8");
}

function sessionKey(url: URL, bitrate: number): string {
  const decodedPath = decodeURIComponent(url.pathname);
  const itemMatch = decodedPath.match(/\/(?:emby\/)?videos\/([^/]+)/i);
  const itemID = itemMatch?.[1] || decodedPath.replace(/[^a-z0-9]/gi, "_").slice(-96);
  return `synctv-emby-play-session:${itemID}:${bitrate}`;
}

function makeVariant(base: URL, bitrate: number): string {
  const variant = new URL(base.toString());
  setSearchParam(variant, "VideoBitrate", String(bitrate));

  if (findSearchParam(variant, "MaxStreamingBitrate")) {
    setSearchParam(variant, "MaxStreamingBitrate", String(bitrate));
  }

  const deviceID = readOrCreate(browserStorage("local"), DEVICE_STORAGE_KEY);
  const playSessionID = readOrCreate(browserStorage("session"), sessionKey(base, bitrate));
  setSearchParam(variant, "DeviceId", deviceID);
  setSearchParam(variant, "PlaySessionId", playSessionID);
  return variant.toString();
}

export function buildEmbyQualityOptions(rawURL: string): EmbyQualityOption[] {
  if (!rawURL || rawURL.startsWith("blob:")) return [];

  let base: URL;
  try {
    base = new URL(rawURL, typeof window === "undefined" ? "http://localhost" : window.location.href);
  } catch {
    return [];
  }

  if (!isConfigurableEmbyPlaylist(base)) return [];

  const currentBitrate = Number(getSearchParam(base, "VideoBitrate"));
  if (!Number.isFinite(currentBitrate) || currentBitrate <= 0) return [];

  const presets = [...QUALITY_PRESETS];
  const hasNearCurrentPreset = presets.some(
    (preset) => Math.abs(preset.bitrate - currentBitrate) / currentBitrate < 0.08
  );
  if (!hasNearCurrentPreset) {
    presets.push({
      name: `当前 ${(currentBitrate / 1_000_000).toFixed(1)} Mbps`,
      bitrate: Math.round(currentBitrate)
    });
  }

  const seen = new Set<number>();
  return presets
    .sort((left, right) => right.bitrate - left.bitrate)
    .filter((preset) => {
      if (seen.has(preset.bitrate)) return false;
      seen.add(preset.bitrate);
      return true;
    })
    .map((preset) => ({
      ...preset,
      url: makeVariant(base, preset.bitrate)
    }));
}

export function embyVideoBitrate(rawURL: string): number | undefined {
  if (!rawURL || rawURL.startsWith("blob:")) return undefined;

  try {
    const url = new URL(rawURL, typeof window === "undefined" ? "http://localhost" : window.location.href);
    const value = Number(getSearchParam(url, "VideoBitrate"));
    return Number.isFinite(value) && value > 0 ? value : undefined;
  } catch {
    return undefined;
  }
}
