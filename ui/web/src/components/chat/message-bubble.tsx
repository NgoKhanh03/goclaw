import { Bot, User, Check, AlertTriangle, Wrench } from "lucide-react";
import { MessageContent } from "./message-content";
import type { ChatMessage } from "@/types/chat";

interface MessageBubbleProps {
  message: ChatMessage;
}

function HistoricToolCallCard({ name, status }: { name: string; status: "completed" | "error" }) {
  return (
    <div className="my-1 flex items-center gap-2 rounded-md border bg-muted/50 px-3 py-2 text-sm">
      {status === "error" ? (
        <AlertTriangle className="h-4 w-4 text-red-500" />
      ) : (
        <Check className="h-4 w-4 text-green-500" />
      )}
      <Wrench className="h-3.5 w-3.5 text-muted-foreground" />
      <span className="font-medium">{name}</span>
      <span className={`ml-auto text-xs ${status === "error" ? "text-red-500" : "text-green-500"}`}>
        {status === "error" ? "Failed" : "Done"}
      </span>
    </div>
  );
}

export function MessageBubble({ message }: MessageBubbleProps) {
  const isUser = message.role === "user";
  const isTool = message.role === "tool";

  if (isTool) {
    return null; // Tool messages are shown inline with assistant messages
  }

  // Assistant message with tool calls but no text content → render tool call cards
  const hasToolCalls = Array.isArray(message.toolCalls) && message.toolCalls.length > 0;
  const hasContent = message.content && message.content.trim().length > 0;

  if (!isUser && hasToolCalls && !hasContent) {
    return (
      <div className="flex gap-3">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border bg-background">
          <Bot className="h-4 w-4" />
        </div>
        <div className="max-w-[80%] space-y-1">
          {message.toolCalls!.map((tc) => (
            <HistoricToolCallCard key={tc.id} name={tc.name} status="completed" />
          ))}
          {message.timestamp && (
            <div className="text-[10px] text-muted-foreground">
              {new Date(message.timestamp).toLocaleTimeString([], {
                hour: "numeric",
                minute: "2-digit",
              })}
            </div>
          )}
        </div>
      </div>
    );
  }

  // Skip rendering if assistant message has neither content nor tool calls
  if (!isUser && !hasContent && !hasToolCalls) {
    return null;
  }

  return (
    <div className={`flex gap-3 ${isUser ? "flex-row-reverse" : ""}`}>
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border bg-background">
        {isUser ? (
          <User className="h-4 w-4" />
        ) : (
          <Bot className="h-4 w-4" />
        )}
      </div>

      <div
        className={`max-w-[80%] rounded-lg px-4 py-2 ${isUser
            ? "bg-primary text-primary-foreground"
            : "bg-muted"
          }`}
      >
        <MessageContent content={message.content} role={message.role} />
        {message.timestamp && (
          <div className={`mt-1 text-[10px] ${isUser ? "text-primary-foreground/60" : "text-muted-foreground"}`}>
            {new Date(message.timestamp).toLocaleTimeString([], {
              hour: "numeric",
              minute: "2-digit",
            })}
          </div>
        )}
      </div>
    </div>
  );
}
