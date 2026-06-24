import { useEffect, useState } from "react";
import { ThumbsDown, ThumbsUp, Trash2 } from "lucide-react";
import type { VideoDetail } from "@/types";
import { formatCount } from "@/lib/format";

type Props = {
  video: VideoDetail;
  onDeleteVideo: () => void;
  deleteSaving?: boolean;
  canDelete?: boolean;
};

/**
 * 视频操作工具条。
 * - 整体是一张浮起的圆角玻璃卡，比上一版的横线分隔更"成体"。
 * - 点赞 + 点踩是两个独立按钮。
 * - 删除是唯一的管理操作，hover 时露出 danger 色。
 *
 * 功能没变：
 * - 后端只有点赞计数接口，点踩仅本地 state。
 * - 失败回滚已经处理。
 */
export function VideoActions({
  video,
  onDeleteVideo,
  deleteSaving,
  canDelete = true,
}: Props) {
  const [likes, setLikes] = useState(video.likes ?? 0);
  const [dislikes, setDislikes] = useState(video.dislikes ?? 0);
  const [bursting, setBursting] = useState(false);
  const [liked, setLiked] = useState(false);
  const [disliked, setDisliked] = useState(false);
  const [likeSubmitted, setLikeSubmitted] = useState(false);

  useEffect(() => {
    setLikes(video.likes ?? 0);
    setDislikes(video.dislikes ?? 0);
    setBursting(false);
    setLiked(false);
    setDisliked(false);
    setLikeSubmitted(false);
  }, [video.id, video.likes, video.dislikes]);

  async function handleLike() {
    if (liked) return;
    setLiked(true);
    setBursting(true);
    window.setTimeout(() => setBursting(false), 320);

    if (disliked) {
      setDisliked(false);
      setDislikes((n) => Math.max(0, n - 1));
    }

    if (likeSubmitted) return;

    setLikeSubmitted(true);
    setLikes((n) => n + 1);

    try {
      const res = await fetch(
        `/api/video/${encodeURIComponent(video.id)}/like`,
        { method: "POST", credentials: "include" }
      );
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = (await res.json()) as { likes: number };
      if (typeof data.likes === "number") {
        setLikes(data.likes);
      }
    } catch {
      setLikes((n) => Math.max(0, n - 1));
      setLiked(false);
      setLikeSubmitted(false);
    }
  }

  function handleDislike() {
    if (disliked) {
      setDisliked(false);
      setDislikes((n) => Math.max(0, n - 1));
      return;
    }
    setDisliked(true);
    setDislikes((n) => n + 1);
    if (liked) {
      setLiked(false);
    }
  }

  return (
    <div className="vd-actions" role="toolbar" aria-label="视频操作">
      <div className="vd-actions__group" role="group" aria-label="点赞和点踩">
        <button
          type="button"
          className={`vd-actions__pill vd-actions__like${
            liked ? " is-active" : ""
          }${bursting ? " is-bursting" : ""}`}
          onClick={handleLike}
          aria-pressed={liked}
          aria-label="点赞"
        >
          <ThumbsUp size={18} fill={liked ? "currentColor" : "none"} />
          <span className="vd-actions__count">{formatCount(likes)}</span>
        </button>
        <button
          type="button"
          className={`vd-actions__pill vd-actions__dislike${
            disliked ? " is-active" : ""
          }`}
          onClick={handleDislike}
          aria-pressed={disliked}
          aria-label="点踩"
        >
          <ThumbsDown size={18} fill={disliked ? "currentColor" : "none"} />
          <span className="vd-actions__count">{formatCount(dislikes)}</span>
        </button>
      </div>

      {canDelete && (
        <button
          type="button"
          className="vd-actions__btn vd-actions__delete"
          onClick={onDeleteVideo}
          disabled={deleteSaving}
          aria-label="删除这个视频"
        >
          <Trash2 size={16} />
          <span>{deleteSaving ? "删除中" : "删除"}</span>
        </button>
      )}
    </div>
  );
}
