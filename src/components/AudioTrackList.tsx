import { CirclePlay, Music2 } from "lucide-react";
import { Link } from "react-router-dom";
import type { AudioItem } from "@/types";
import { formatCount } from "@/lib/format";

function formatBytes(bytes: number): string {
  if (bytes <= 0) return "--";
  if (bytes >= 1 << 30) return `${(bytes / (1 << 30)).toFixed(1)} GB`;
  if (bytes >= 1 << 20) return `${(bytes / (1 << 20)).toFixed(1)} MB`;
  return `${Math.max(1, Math.round(bytes / 1024))} KB`;
}

export function AudioTrackList({ items }: { items: AudioItem[] }) {
  return (
    <ol className="audio-track-list" aria-label="音频列表">
      {items.map((audio, index) => (
        <li key={audio.id} className="audio-track">
          <Link className="audio-track__link" to={audio.href}>
            <span className="audio-track__index">{String(index + 1).padStart(2, "0")}</span>
            <span className="audio-track__format" data-format={audio.ext.toLowerCase()}>
              <Music2 size={20} aria-hidden="true" />
              <small>{audio.ext || "audio"}</small>
            </span>
            <span className="audio-track__identity">
              <strong title={audio.title}>{audio.title}</strong>
              <span>{audio.author || audio.sourceLabel || "未知作者"}</span>
            </span>
            <span className="audio-track__stat">{audio.duration}</span>
            <span className="audio-track__stat">{formatBytes(audio.size)}</span>
            <span className="audio-track__stat">{formatCount(audio.views)} 次播放</span>
            <CirclePlay className="audio-track__play" size={24} aria-label="播放" />
          </Link>
        </li>
      ))}
    </ol>
  );
}