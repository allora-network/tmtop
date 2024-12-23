import { useState, useCallback, useEffect, useRef, forwardRef } from 'react'
import { fetchTopologyJSON, ComputeTopologyParams, fetchPeers, JSONResponse, RPC, Conn } from '@/lib/api'
import { GraphCanvas, GraphCanvasRef, InternalGraphNode, InternalGraphEdge, InternalGraphPosition, layoutProvider, recommendLayout, useSelection } from 'reagraph'
import forceAtlas2 from 'graphology-layout-forceatlas2'
import random from 'graphology-layout/random'
import circular from 'graphology-layout/circular'
import Graph from 'graphology'

import './App.css'

function App() {
    const [params, setParams] = useState({})

    const [graph, setGraph] = useState<{ nodes: InternalGraphNode[], edges: InternalGraphEdge[] }>({ nodes: [], edges: [] })
    const [apiData, setAPIData] = useState<JSONResponse>({ nodes: [], conns: [] })
    const [clickedEdges, setClickedEdges] = useState<Record<string, boolean>>({})
    const [minBytesSec, setMinBytesSec] = useState<Number | null>(null)
    const [selectedNode, setSelectedNode] = useState<RPC | null>(null)
    const nodeRef = useRef(new Map<string, InternalGraphPosition>())
    const graphRef = useRef<GraphCanvasRef | null>(null)

    const renderGraph = useCallback(async (selectedPeers: { [id: string]: boolean }, crawlDistance: number) => {
        setMinBytesSec(minBytesSec)
        const graph = await fetchTopologyJSON({
            ...params,
            crawlDistance,
            minBytesSec: minBytesSec || null,
            includeNodes: Object.keys(selectedPeers).filter(url => selectedPeers[url]),
        })
        setAPIData(graph)
    }, [params, fetchTopologyJSON, setAPIData, minBytesSec])

    useEffect(() => {
        let max = (apiData.conns || []).reduce((max, conn) => {
            let total = conn.connectionStatus.send_monitor.avg_rate + conn.connectionStatus.recv_monitor.avg_rate
            if (total > max) {
                return total
            }
            return max
        }, 0)

        const nodes = (apiData.nodes || []).map(node => ({
            id: node.id,
            label: node.validatorMoniker !== '' ? node.validatorMoniker : node.moniker,
            fill: node.validatorAddress !== '' ? 'red' : undefined,
        }))

        const edges = (apiData.conns || [])
            .filter(conn => conn.connectionStatus.send_monitor.avg_rate + conn.connectionStatus.recv_monitor.avg_rate >= (minBytesSec || 0).valueOf())
            .map(conn => ({
                source: conn.from,
                target: conn.to,
                id: `${conn.from}-${conn.to}`,
                label: clickedEdges[`${conn.from}-${conn.to}`] ? `${humanizeBytes(conn.connectionStatus.send_monitor.avg_rate)}/s\n${humanizeBytes(conn.connectionStatus.recv_monitor.avg_rate)}/s` : '',
                size: (conn.connectionStatus.send_monitor.avg_rate + conn.connectionStatus.recv_monitor.avg_rate) / max * 5,
            }))

        let copy = [...nodes]

        let circleSizes = [10, 25, 50, 75, 130, 200, 350, 500]
        let cohorts = []
        let k = 0
        for (let size of circleSizes) {
            const graph = new Graph()
            let i = 0
            for (let node of copy) {
                if (!graph.hasNode(node.id)) {
                    graph.addNode(node.id)
                    i++
                }

                if (i >= size) {
                    copy = copy.slice(i)
                    break
                }
            }
            random.assign(graph)

            let positions = circular(graph, { scale: size * (8-k) })
            console.log(positions)

            let j = 0
            for (let id in positions) {
                let position = positions[id]
                nodeRef.current.set(id, { id, x: position.x, vx: position.x, y: position.y, vy: position.y, z: 1, links: [], data: null, index: j++ })
            }
            // for (let edge of edges) {
            //     graph.addEdge(edge.source, edge.target)
            // }
            k++
        }

        setGraph({ nodes, edges })
    }, [apiData, clickedEdges, setGraph, minBytesSec])

    let { nodes, edges } = graph

    function onEdgeClick(edge: InternalGraphEdge) {
        setClickedEdges({ ...clickedEdges, [edge.id]: !clickedEdges[edge.id] })
    }

    function handleNodeClick(node: InternalGraphNode) {
        const selectedRPC = (apiData.nodes || []).find(n => n.id === node.id)
        if (selectedRPC) {
            const connectedPeers = (apiData.conns || [])
                .filter(conn => conn.from === selectedRPC.id || conn.to === selectedRPC.id)
                .map(conn => {
                    const peerId = conn.from === selectedRPC.id ? conn.to : conn.from
                    const peer = (apiData.nodes || []).find(n => n.id === peerId)
                    return {
                        peer,
                        connectionStatus: conn.connectionStatus,
                        isOutbound: conn.from === selectedRPC.id
                    }
                })
            setSelectedNode({ ...selectedRPC, connectedPeers })
        } else {
            setSelectedNode(null)
        }
    }

    function handleCanvasClick() {
        setSelectedNode(null)
    }

    const {
        selections,
        actives,
        onNodeClick: selectionNodeClick,
        onCanvasClick: selectionCanvasClick
    } = useSelection({
        ref: graphRef,
        nodes: nodes,
        edges: edges,
        pathSelectionType: 'all'
    })

    return (
        <>
            <div style={{ width: '100vw', height: '100vh' }}>
                <Sidebar renderGraph={renderGraph} minBytesSec={minBytesSec} setMinBytesSec={setMinBytesSec} />
                <div>
                    <GraphCanvas
                        ref={graphRef}
                        nodes={nodes}
                        edges={edges}
                        labelType="all"
                        draggable
                        layoutType='forceDirected2d'
                        layoutOverrides={{
                            getNodePosition: (id, nodePositionArgs) => {
                                let idx = nodes.findIndex(node => node.id === id)
                                if (idx === -1) {
                                    idx = Math.random() * 100
                                }

                                const position = {
                                    x: 25 * idx,
                                    y: idx % 2 === 0 ? 0 : 50,
                                    z: 1,
                                }

                                return nodeRef.current?.get(id) || (function() {
                                    // This next bit is quite fraught -- do not modify unless you know what you're doing
                                    nodeRef.current.set(id, { id, x: position.x, vx: position.x, y: position.y, vy: position.y, z: 1, links: [], data: null, index: idx })
                                    return position
                                })()
                            },
                        }}
                        onNodeDragged={node => {
                            nodeRef.current.set(node.id, node.position)
                        }}
                        onEdgeClick={onEdgeClick}
                        selections={selections}
                        actives={actives}
                        onCanvasClick={(event) => {
                            handleCanvasClick()
                            selectionCanvasClick(event)
                        }}
                        onNodeClick={(node, event) => {
                            handleNodeClick(node)
                            selectionNodeClick(node, event)
                        }}
                    />
                </div>
                {selectedNode && (
                    <div style={{
                        position: 'absolute',
                        top: 0,
                        right: 0,
                        width: '450px',
                        height: '100vh',
                        backgroundColor: 'rgba(36, 36, 36, 0.9)',
                        padding: '20px',
                        overflowY: 'auto'
                    }}><NodeDetails node={selectedNode} /></div>
                )}
            </div>
        </>
    )
}

function Sidebar(props: {
    minBytesSec: number,
    setMinBytesSec: (x: number) => void,
    renderGraph: (selectedPeers: { [id: string]: boolean }, crawlDistance: number) => void,
}) {
    const { renderGraph, minBytesSec, setMinBytesSec } = props
    const [peers, setPeers] = useState<RPC[]>([])
    const [selectedPeers, setSelectedPeers] = useState<{ [url: string]: boolean }>({})
    const [sidebarOpen, setSidebarOpen] = useState(true)
    const [filterText, setFilterText] = useState('')
    const [crawlDistance, setCrawlDistance] = useState(1)

    useEffect(() => {
        (async function() {
            let peers = await fetchPeers()
            setPeers(peers)
        })()
    }, [])

    const filteredPeers = peers.filter(peer =>
        peer.moniker.toLowerCase().includes(filterText.toLowerCase()) ||
        peer.id.toLowerCase().includes(filterText.toLowerCase()) ||
        peer.url.toLowerCase().includes(filterText.toLowerCase())
    )

    return (
        <>
            <div style={{ width: sidebarOpen ? 'fit-content' : 0, height: '100vh', overflowX: 'scroll', position: 'absolute', top: 0, left: 0, zIndex: 9999, backgroundColor: '#242424' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                    <button style={{ margin: 8 }} onClick={() => renderGraph(selectedPeers, crawlDistance)}>Generate</button>
                    <input
                        type="text"
                        placeholder="Filter peers..."
                        value={filterText}
                        onChange={(e) => setFilterText(e.target.value)}
                        style={{ width: 'calc(100% - 88px)', height: '1rem', marginTop: 8, marginRight: 10, padding: '5px' }}
                    />
                    <input
                        type="text"
                        placeholder="Crawl..."
                        value={crawlDistance}
                        onChange={(e) => setCrawlDistance(Number(e.target.value))}
                        style={{ width: 64, height: '1rem', marginTop: 8, marginRight: 10, padding: '5px' }}
                    />
                    <input
                        type="text"
                        placeholder="Min bytes/sec..."
                        value={minBytesSec}
                        onChange={(e) => setMinBytesSec(Number(e.target.value))}
                        style={{ width: 128, height: '1rem', marginTop: 8, padding: '5px' }}
                    />
                    <button onClick={() => setSidebarOpen(false)} style={{ margin: 8, zIndex: 9 }}>Close</button>
                </div>
                <table>
                    {filteredPeers.map(peer => (
                        <tr key={peer.url}>
                            <td>
                                <input
                                    type="checkbox"
                                    checked={selectedPeers[peer.url]}
                                    onChange={e => setSelectedPeers({ ...selectedPeers, [peer.url]: e.target.checked })} />
                            </td>
                            <td>{peer.moniker}</td>
                            <td>{peer.id}</td>
                            <td>{peer.url}</td>
                        </tr>
                    ))}
                </table>
            </div>
            <button onClick={() => setSidebarOpen(true)} style={{ position: 'absolute', top: 16, left: 16, zIndex: 9 }}>Open sidebar</button>
        </>
    )
}

type NodeWithPeers = RPC & {
    connectedPeers: Array<{
        peer: RPC | undefined,
        connectionStatus: Conn['connectionStatus'],
        isOutbound: boolean
    }>
}

function NodeDetails({ node }: { node: NodeWithPeers }) {
    return (
        <div>
            <h2>{node.moniker}</h2>
            <table>
                <tbody>
                    <tr><td>ID:</td><td>{node.id}</td></tr>
                    <tr><td>URL:</td><td>{node.url}</td></tr>
                    <tr><td>IP:</td><td>{node.ip}</td></tr>
                    <tr><td>Validator Moniker:</td><td>{node.validatorMoniker || 'N/A'}</td></tr>
                    <tr><td>Validator Address:</td><td>{node.validatorAddress || 'N/A'}</td></tr>
                    {Object.entries(node).map(([key, value]) => {
                        if (typeof value !== 'object' && !['id', 'url', 'ip', 'moniker', 'validatorMoniker', 'validatorAddress'].includes(key)) {
                            return (
                                <tr key={key}>
                                    <td>{key}:</td>
                                    <td>{String(value)}</td>
                                </tr>
                            )
                        }
                        return null
                    })}
                </tbody>
            </table>
            <h3>Connected Peers</h3>
            <table>
                <thead>
                    <tr>
                        <th>Moniker</th>
                        <th>Direction</th>
                        <th>Send Rate</th>
                        <th>Receive Rate</th>
                    </tr>
                </thead>
                <tbody>
                    {node.connectedPeers.map((peerInfo, index) => (
                        <tr key={index}>
                            <td>{peerInfo.peer?.moniker || 'Unknown'}</td>
                            <td>{peerInfo.isOutbound ? 'Outbound' : 'Inbound'}</td>
                            <td>{humanizeBytes(peerInfo.connectionStatus.send_monitor.avg_rate)}/s</td>
                            <td>{humanizeBytes(peerInfo.connectionStatus.recv_monitor.avg_rate)}/s</td>
                        </tr>
                    ))}
                </tbody>
            </table>
            <h3>Connection Statistics</h3>
            <table>
                <tbody>
                    <tr><td>Total Send Rate:</td><td>{humanizeBytes(node.connectedPeers.reduce((sum, peer) => sum + peer.connectionStatus.send_monitor.avg_rate, 0))}/s</td></tr>
                    <tr><td>Total Receive Rate:</td><td>{humanizeBytes(node.connectedPeers.reduce((sum, peer) => sum + peer.connectionStatus.recv_monitor.avg_rate, 0))}/s</td></tr>
                    <tr><td>Outbound Connections:</td><td>{node.connectedPeers.filter(peer => peer.isOutbound).length}</td></tr>
                    <tr><td>Inbound Connections:</td><td>{node.connectedPeers.filter(peer => !peer.isOutbound).length}</td></tr>
                </tbody>
            </table>
        </div>
    )
}

function humanizeBytes(bytes: number | string, si = true, dp = 1) {
    bytes = Number(bytes)
    const thresh = si ? 1000 : 1024

    if (Math.abs(bytes) < thresh) {
        return bytes + ' B'
    }

    const units = si
        ? ['kB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB']
        : ['KiB', 'MiB', 'GiB', 'TiB', 'PiB', 'EiB', 'ZiB', 'YiB']
    let u = -1
    const r = 10**dp

    do {
        bytes /= thresh
        ++u
    } while (Math.round(Math.abs(bytes) * r) / r >= thresh && u < units.length - 1)

    return bytes.toFixed(dp) + ' ' + units[u]
}

export default App