import { useState } from "react";
import { Bot, MessageSquareWarning, ThumbsDown, ThumbsUp, User } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { MessageContent } from "./message-content";
import { ThinkingBlock } from "./thinking-block";
import { ToolCallCard } from "./tool-call-card";
import { BlockReplyBubble } from "./block-reply-bubble";
import { MediaGallery } from "./media-gallery";
import { useUiStore } from "@/stores/use-ui-store";
import { resolveTimezone } from "@/lib/format";
import type { ChatMessage } from "@/types/chat";

interface MessageBubbleProps {
  message: ChatMessage;
  agentId?: string;
  sessionKey?: string;
  messageRef?: string;
  onFeedback?: (input: {
    feedback_type: "useful" | "not_useful" | "correction";
    session_key: string;
    message_ref: string;
    message_content?: string;
    correction?: string;
  }) => Promise<void>;
}

export function MessageBubble({ message, agentId, sessionKey, messageRef, onFeedback }: MessageBubbleProps) {
  const timezone = useUiStore((s) => s.timezone);
  const [feedbackSent, setFeedbackSent] = useState<string | null>(null);
  const [correctionOpen, setCorrectionOpen] = useState(false);
  const [correction, setCorrection] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const isUser = message.role === "user";
  const isTool = message.role === "tool";

  if (isTool) return null;
  if (message.isNotification) return null;
  if (message.isBlockReply) return <BlockReplyBubble message={message} />;

  const isAssistant = message.role === "assistant";
  const hasThinking = isAssistant && !!message.thinking;
  const hasToolDetails = isAssistant && message.toolDetails && message.toolDetails.length > 0;
  const hasToolCalls = isAssistant && message.tool_calls && message.tool_calls.length > 0;
  const hasContent = !!message.content?.trim();

  if (isAssistant && !hasContent && !hasToolCalls && !hasToolDetails) return null;

  const canFeedback = isAssistant && !!agentId && !!sessionKey && !!messageRef && !!onFeedback && hasContent;

  const submitFeedback = async (type: "useful" | "not_useful" | "correction") => {
    if (!canFeedback || submitting) return;
    if (type === "correction" && !correction.trim()) {
      setCorrectionOpen(true);
      return;
    }
    setSubmitting(true);
    try {
      await onFeedback({
        feedback_type: type,
        session_key: sessionKey,
        message_ref: messageRef,
        message_content: message.content,
        correction: type === "correction" ? correction.trim() : undefined,
      });
      setFeedbackSent(type);
      if (type === "correction") {
        setCorrection("");
        setCorrectionOpen(false);
      }
    } finally {
      setSubmitting(false);
    }
  };

  const isToolOnly = isAssistant && !hasContent && !hasThinking && (hasToolDetails || hasToolCalls);

  return (
    <div className={`flex gap-3 ${isUser ? "flex-row-reverse" : ""}`}>
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border bg-background">
        {isUser ? <User className="h-4 w-4" /> : <Bot className="h-4 w-4" />}
      </div>

      {isToolOnly ? (
        <div className="flex-1 min-w-0 rounded-md border bg-muted divide-y divide-border">
          {hasThinking && (
            <div className="px-2 py-1.5">
              <ThinkingBlock text={message.thinking!} />
            </div>
          )}
          {hasToolDetails && message.toolDetails!.map((entry) => (
            <ToolCallCard key={entry.toolCallId} entry={entry} compact />
          ))}
        </div>
      ) : (
        <div className={`rounded-lg px-4 py-2 ${
          isUser
            ? "max-w-[85%] bg-card text-card-foreground border border-border shadow-sm border-r-2 border-r-accent-foreground"
            : "flex-1 min-w-0 bg-card text-card-foreground border border-border shadow-sm"
        }`}>
          {hasThinking && (
            <div className="mb-2">
              <ThinkingBlock text={message.thinking!} />
            </div>
          )}
          {hasToolDetails && (
            <div className="mb-2 rounded-md border bg-muted divide-y divide-border">
              {message.toolDetails!.map((entry) => (
                <ToolCallCard key={entry.toolCallId} entry={entry} compact />
              ))}
            </div>
          )}
          <MessageContent content={message.content} role={message.role} mediaBasenames={message.mediaItems?.map((m) => m.path.split("/").pop() ?? "").filter(Boolean)} />
          {message.mediaItems && message.mediaItems.length > 0 && (
            <div className="mt-2">
              <MediaGallery items={message.mediaItems} />
            </div>
          )}
          {message.timestamp && (
            <div className="mt-1 text-2xs text-muted-foreground">
              {new Intl.DateTimeFormat([], {
                timeZone: resolveTimezone(timezone),
                hour: "numeric",
                minute: "2-digit",
              }).format(new Date(message.timestamp))}
            </div>
          )}
          {canFeedback && (
            <div className="mt-2 border-t pt-2">
              <div className="flex flex-wrap items-center gap-1">
                <Button
                  type="button"
                  size="sm"
                  variant={feedbackSent === "useful" ? "secondary" : "ghost"}
                  className="h-7 px-2 text-xs"
                  disabled={submitting}
                  onClick={() => submitFeedback("useful")}
                >
                  <ThumbsUp className="mr-1 h-3 w-3" />
                  有用
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant={feedbackSent === "not_useful" ? "secondary" : "ghost"}
                  className="h-7 px-2 text-xs"
                  disabled={submitting}
                  onClick={() => submitFeedback("not_useful")}
                >
                  <ThumbsDown className="mr-1 h-3 w-3" />
                  没用
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant={correctionOpen || feedbackSent === "correction" ? "secondary" : "ghost"}
                  className="h-7 px-2 text-xs"
                  disabled={submitting}
                  onClick={() => setCorrectionOpen((v) => !v)}
                >
                  <MessageSquareWarning className="mr-1 h-3 w-3" />
                  纠错
                </Button>
                {feedbackSent && <span className="text-xs text-muted-foreground">已记录</span>}
              </div>
              {correctionOpen && (
                <div className="mt-2 space-y-2">
                  <Textarea
                    value={correction}
                    onChange={(event) => setCorrection(event.target.value)}
                    placeholder="写下正确做法或这次回复的问题。管理员会在智能进化中心审核。"
                    className="min-h-20 text-sm"
                  />
                  <div className="flex justify-end gap-2">
                    <Button type="button" size="sm" variant="outline" onClick={() => setCorrectionOpen(false)}>
                      取消
                    </Button>
                    <Button type="button" size="sm" disabled={submitting || !correction.trim()} onClick={() => submitFeedback("correction")}>
                      提交纠错
                    </Button>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
