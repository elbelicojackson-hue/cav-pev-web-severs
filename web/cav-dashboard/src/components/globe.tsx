"use client";

import { useRef, useMemo, useState } from "react";
import { Canvas, useFrame } from "@react-three/fiber";
import { Sphere, Line, Float } from "@react-three/drei";
import * as THREE from "three";

// Generate points on a sphere surface (Fibonacci distribution)
function fibonacciSphere(samples: number, radius: number): THREE.Vector3[] {
  const points: THREE.Vector3[] = [];
  const phi = Math.PI * (Math.sqrt(5) - 1);
  for (let i = 0; i < samples; i++) {
    const y = 1 - (i / (samples - 1)) * 2;
    const r = Math.sqrt(1 - y * y);
    const theta = phi * i;
    points.push(new THREE.Vector3(Math.cos(theta) * r * radius, y * radius, Math.sin(theta) * r * radius));
  }
  return points;
}

// Generate connections between nearby points
function generateConnections(points: THREE.Vector3[], maxDist: number, maxCount: number): [THREE.Vector3, THREE.Vector3][] {
  const connections: [THREE.Vector3, THREE.Vector3][] = [];
  for (let i = 0; i < points.length; i++) {
    for (let j = i + 1; j < points.length; j++) {
      if (points[i].distanceTo(points[j]) < maxDist && connections.length < maxCount) {
        connections.push([points[i], points[j]]);
      }
    }
  }
  return connections;
}

// Animated data pulse traveling along a connection
function DataPulse({ start, end, speed, color }: { start: THREE.Vector3; end: THREE.Vector3; speed: number; color: string }) {
  const meshRef = useRef<THREE.Mesh>(null);
  const progress = useRef(Math.random());

  useFrame((_, delta) => {
    progress.current += delta * speed;
    if (progress.current > 1) progress.current = 0;
    if (meshRef.current) {
      meshRef.current.position.lerpVectors(start, end, progress.current);
      // Pulse scale based on position
      const scale = Math.sin(progress.current * Math.PI) * 1.5 + 0.5;
      meshRef.current.scale.setScalar(scale);
    }
  });

  return (
    <mesh ref={meshRef}>
      <sphereGeometry args={[0.015, 8, 8]} />
      <meshBasicMaterial color={color} transparent opacity={0.9} />
    </mesh>
  );
}

// Orbiting ring
function OrbitalRing({ radius, tilt, speed, color, opacity }: { radius: number; tilt: number; speed: number; color: string; opacity: number }) {
  const ringRef = useRef<THREE.Group>(null);

  useFrame((_, delta) => {
    if (ringRef.current) {
      ringRef.current.rotation.z += delta * speed;
    }
  });

  const points = useMemo(() => {
    const pts: THREE.Vector3[] = [];
    for (let i = 0; i <= 128; i++) {
      const angle = (i / 128) * Math.PI * 2;
      pts.push(new THREE.Vector3(Math.cos(angle) * radius, Math.sin(angle) * radius, 0));
    }
    return pts;
  }, [radius]);

  return (
    <group ref={ringRef} rotation={[tilt, 0, 0]}>
      <Line points={points} color={color} lineWidth={1} transparent opacity={opacity} />
    </group>
  );
}

// Floating particle field around the globe
function ParticleField({ count, radius }: { count: number; radius: number }) {
  const meshRef = useRef<THREE.InstancedMesh>(null);
  const dummy = useMemo(() => new THREE.Object3D(), []);
  const speeds = useMemo(() => Array.from({ length: count }, () => 0.1 + Math.random() * 0.3), [count]);
  const offsets = useMemo(() => Array.from({ length: count }, () => Math.random() * Math.PI * 2), [count]);
  const radii = useMemo(() => Array.from({ length: count }, () => radius + Math.random() * 1.5), [count, radius]);

  useFrame((state) => {
    if (!meshRef.current) return;
    const time = state.clock.elapsedTime;
    for (let i = 0; i < count; i++) {
      const angle = time * speeds[i] + offsets[i];
      const r = radii[i];
      const y = Math.sin(angle * 0.5 + offsets[i]) * r * 0.6;
      dummy.position.set(Math.cos(angle) * r, y, Math.sin(angle) * r);
      dummy.scale.setScalar(0.3 + Math.sin(time * 2 + offsets[i]) * 0.2);
      dummy.updateMatrix();
      meshRef.current.setMatrixAt(i, dummy.matrix);
    }
    meshRef.current.instanceMatrix.needsUpdate = true;
  });

  return (
    <instancedMesh ref={meshRef} args={[undefined, undefined, count]}>
      <sphereGeometry args={[0.012, 6, 6]} />
      <meshBasicMaterial color="#60a5fa" transparent opacity={0.4} />
    </instancedMesh>
  );
}

function GlobeScene() {
  const groupRef = useRef<THREE.Group>(null);
  const points = useMemo(() => fibonacciSphere(120, 2.2), []);
  const connections = useMemo(() => generateConnections(points, 1.0, 180), [points]);
  const [hovered] = useState(false);

  useFrame((_, delta) => {
    if (groupRef.current) {
      groupRef.current.rotation.y += delta * (hovered ? 0.02 : 0.06);
    }
  });

  // Select some connections for data pulses
  const pulseConnections = useMemo(() => connections.filter((_, i) => i % 8 === 0), [connections]);

  return (
    <>
      <group ref={groupRef}>
        {/* Inner glow sphere */}
        <Sphere args={[2.0, 48, 48]}>
          <meshBasicMaterial color="#0a1628" transparent opacity={0.6} />
        </Sphere>

        {/* Wireframe sphere - outer shell */}
        <Sphere args={[2.15, 48, 48]}>
          <meshBasicMaterial color="#1e3a5f" wireframe transparent opacity={0.08} />
        </Sphere>

        {/* Atmosphere glow */}
        <Sphere args={[2.35, 48, 48]}>
          <meshBasicMaterial color="#3b82f6" transparent opacity={0.03} side={THREE.BackSide} />
        </Sphere>

        {/* Node points */}
        {points.map((point, i) => {
          const isHighlight = i % 7 === 0;
          const isMajor = i % 15 === 0;
          return (
            <mesh key={i} position={point}>
              <sphereGeometry args={[isMajor ? 0.04 : isHighlight ? 0.03 : 0.015, 8, 8]} />
              <meshBasicMaterial
                color={isMajor ? "#a78bfa" : isHighlight ? "#60a5fa" : "#475569"}
                transparent
                opacity={isMajor ? 1 : isHighlight ? 0.8 : 0.5}
              />
            </mesh>
          );
        })}

        {/* Connection lines */}
        {connections.map(([start, end], i) => (
          <Line
            key={i}
            points={[start, end]}
            color={i % 12 === 0 ? "#a78bfa" : "#3b82f6"}
            lineWidth={i % 12 === 0 ? 1 : 0.5}
            transparent
            opacity={i % 12 === 0 ? 0.4 : 0.15}
          />
        ))}

        {/* Data pulses traveling along connections */}
        {pulseConnections.map(([start, end], i) => (
          <DataPulse
            key={`pulse-${i}`}
            start={start}
            end={end}
            speed={0.3 + Math.random() * 0.5}
            color={i % 3 === 0 ? "#a78bfa" : "#60a5fa"}
          />
        ))}
      </group>

      {/* Orbital rings */}
      <OrbitalRing radius={2.8} tilt={1.2} speed={0.15} color="#3b82f6" opacity={0.15} />
      <OrbitalRing radius={3.1} tilt={-0.8} speed={-0.1} color="#a78bfa" opacity={0.1} />
      <OrbitalRing radius={3.4} tilt={0.4} speed={0.08} color="#60a5fa" opacity={0.06} />

      {/* Floating particles */}
      <ParticleField count={60} radius={2.8} />

      {/* Floating satellite nodes */}
      <Float speed={1.5} rotationIntensity={0.2} floatIntensity={0.5}>
        <mesh position={[3.2, 1.5, 0.5]}>
          <octahedronGeometry args={[0.08]} />
          <meshBasicMaterial color="#a78bfa" />
        </mesh>
      </Float>
      <Float speed={2} rotationIntensity={0.3} floatIntensity={0.8}>
        <mesh position={[-2.8, -1.2, 1.5]}>
          <octahedronGeometry args={[0.06]} />
          <meshBasicMaterial color="#60a5fa" />
        </mesh>
      </Float>
      <Float speed={1.2} rotationIntensity={0.1} floatIntensity={0.6}>
        <mesh position={[1.5, -2.5, -1.8]}>
          <octahedronGeometry args={[0.07]} />
          <meshBasicMaterial color="#34d399" />
        </mesh>
      </Float>
    </>
  );
}

export function Globe({ className }: { className?: string }) {
  return (
    <div className={className}>
      <Canvas
        camera={{ position: [0, 0, 6.5], fov: 40 }}
        gl={{ antialias: true, alpha: true }}
        style={{ background: "transparent" }}
      >
        <GlobeScene />
      </Canvas>
    </div>
  );
}
