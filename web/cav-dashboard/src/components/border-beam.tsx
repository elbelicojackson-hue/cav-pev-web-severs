"use client";

interface BorderBeamProps {
  duration?: number;
  delay?: number;
  color?: string;
  size?: number;
}

export function BorderBeam({
  duration = 6,
  delay = 0,
  color = "rgba(59, 130, 246, 0.8)",
  size = 150,
}: BorderBeamProps) {
  return (
    <div className="absolute inset-0 overflow-hidden rounded-xl pointer-events-none">
      <div
        className="absolute"
        style={{
          width: `${size}px`,
          height: `${size}px`,
          background: `radial-gradient(circle, ${color} 0%, transparent 70%)`,
          animation: `border-beam ${duration}s linear ${delay}s infinite`,
        }}
      />
      <style jsx>{`
        @keyframes border-beam {
          0% { top: -${size / 2}px; left: -${size / 2}px; }
          25% { top: -${size / 2}px; left: calc(100% - ${size / 2}px); }
          50% { top: calc(100% - ${size / 2}px); left: calc(100% - ${size / 2}px); }
          75% { top: calc(100% - ${size / 2}px); left: -${size / 2}px; }
          100% { top: -${size / 2}px; left: -${size / 2}px; }
        }
      `}</style>
    </div>
  );
}
