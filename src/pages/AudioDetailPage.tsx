import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ArrowLeft, Headphones, Music2 } from "lucide-react";
import { AppShell } from "@/components/AppShell";
import { AudioPlayer } from "@/components/AudioPlayer";
import { fetchAudioDetail, recordAudioView } from "@/data/audios";
import type { AudioDetail } from "@/types";

export default function AudioDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [detail, setDetail] = useState<AudioDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);
  const [unsupported, setUnsupported] = useState(false);

  useEffect(() => {
    if (!id) return;
    let active = true;
    setLoading(true);
    setFailed(false);
    setUnsupported(false);
    fetchAudioDetail(id)
      .then((item) => {
        if (!active) return;
        setDetail(item);
        document.title = item.title;
      })
      .catch(() => {
        if (active) setFailed(true);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => { active = false; };
  }, [id]);

  return (
    <AppShell>
      <div className="container audio-detail">
        <Link to="/audio" className="audio-detail__back"><ArrowLeft size={17} /> 返回音频库</Link>
        {loading ? (
          <div className="audio-library__state">音频加载中...</div>
        ) : failed || !detail ? (
          <div className="audio-library__state is-error">音频不存在或加载失败</div>
        ) : (
          <>
            <header className="audio-detail__head">
              <span className="audio-detail__mark"><Music2 size={32} /></span>
              <div>
                <span className="audio-library__eyebrow">{detail.ext.toUpperCase()} · {detail.sourceLabel || "音频"}</span>
                <h1>{detail.title}</h1>
                <p>{detail.author || "未知作者"}</p>
              </div>
            </header>
            <AudioPlayer
              src={detail.audioSrc}
              title={detail.title}
              ext={detail.ext}
              onFirstPlay={() => recordAudioView(detail.id).catch(() => undefined)}
              onUnsupported={() => setUnsupported(true)}
            />
            {unsupported && (
              <div className="audio-detail__warning" role="alert">
                当前浏览器可能不支持此音频格式（{detail.ext.toUpperCase()}），请尝试其他浏览器。
              </div>
            )}
            <section className="audio-detail__meta">
              <span><Headphones size={16} /> {detail.views} 次播放</span>
              <span>时长 {detail.duration}</span>
              <span>收录于 {detail.publishedAt}</span>
              {detail.tags.map((tag) => <span className="audio-detail__tag" key={tag}>{tag}</span>)}
            </section>
            {detail.relatedAudios.length > 0 && (
              <section className="audio-detail__related">
                <h2>继续聆听</h2>
                <ul>
                  {detail.relatedAudios.map((item) => (
                    <li key={item.id}>
                      <Link to={item.href}><Music2 size={18} /><span><strong>{item.title}</strong><small>{item.author || item.ext.toUpperCase()}</small></span></Link>
                    </li>
                  ))}
                </ul>
              </section>
            )}
          </>
        )}
      </div>
    </AppShell>
  );
}