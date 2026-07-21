import { useRef, useState } from "react";
import { Disc3 } from "lucide-react";

type Props = {
  src: string;
  title: string;
  ext: string;
  onFirstPlay: () => void;
  onUnsupported: () => void;
};

export function AudioPlayer({ src, title, ext, onFirstPlay, onUnsupported }: Props) {
  const played = useRef(false);
  const [active, setActive] = useState(false);
  return (
    <section className={`audio-player ${active ? "is-playing" : ""}`} aria-label={`${title} 播放器`}>
      <div className="audio-player__disc" aria-hidden="true">
        <Disc3 size={56} />
        <span>{ext.toUpperCase()}</span>
      </div>
      <div className="audio-player__body">
        <strong>{title}</strong>
        <audio
          controls
          preload="metadata"
          src={src}
          onPlay={() => {
            setActive(true);
            if (!played.current) {
              played.current = true;
              onFirstPlay();
            }
          }}
          onPause={() => setActive(false)}
          onEnded={() => setActive(false)}
          onError={onUnsupported}
        />
      </div>
    </section>
  );
}