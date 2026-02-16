"""
Example: Instrumenting an AI agent with Oculo.

This script demonstrates how to use the Oculo Python SDK
to trace an AI agent's execution, including LLM calls,
tool invocations, and memory mutations.

Prerequisites:
  1. Start the Oculo daemon: oculo-daemon
  2. Install the SDK: cd sdk/python && pip install -e .
  3. Run this script: python examples/sample_agent.py
  4. View traces in the TUI: oculo-tui
"""

import time
import random
from oculo import OculoTracer


def simulate_llm_call(prompt: str) -> dict:
    """Simulate an LLM API call with fake latency and response."""
    time.sleep(random.uniform(0.1, 0.3))  # Simulate network latency
    
    responses = {
        "research": "Based on my analysis, transformer architectures use self-attention mechanisms...",
        "plan": "I recommend the following approach: 1) Gather data, 2) Analyze patterns, 3) Report findings.",
        "summarize": "Key findings: Attention is all you need. Multi-head attention enables parallel processing.",
    }
    
    # Pick a response based on keywords
    for keyword, response in responses.items():
        if keyword in prompt.lower():
            return {
                "text": response,
                "prompt_tokens": len(prompt.split()) * 2,
                "completion_tokens": len(response.split()) * 2,
                "model": "gpt-4-turbo",
            }
    
    default_response = "I'll help you with that. Let me analyze the information..."
    return {
        "text": default_response,
        "prompt_tokens": len(prompt.split()) * 2,
        "completion_tokens": len(default_response.split()) * 2,
        "model": "gpt-4-turbo",
    }


def simulate_search_tool(query: str) -> dict:
    """Simulate a web search tool call."""
    time.sleep(random.uniform(0.05, 0.15))
    return {
        "results": [
            {"title": f"Result about {query}", "url": f"https://example.com/{query}"},
            {"title": f"More info on {query}", "url": f"https://papers.example.com/{query}"},
        ],
        "total_results": 42,
    }


def main():
    print("🔍 Starting sample agent with Oculo instrumentation...\n")

    # Initialize the tracer
    with OculoTracer(
        agent_name="research-agent-v1",
        metadata={"version": "1.0", "environment": "development"},
    ) as tracer:

        # Create a trace for this agent run
        with tracer.trace() as trace:

            # ─── Step 1: Planning ───
            print("📋 Step 1: Planning...")
            with trace.span("planning_step", operation_type="PLANNING") as span:
                prompt = "Create a research plan for understanding transformer architectures"
                result = simulate_llm_call(prompt)
                
                span.set_prompt(prompt)
                span.set_completion(
                    result["text"],
                    prompt_tokens=result["prompt_tokens"],
                    completion_tokens=result["completion_tokens"],
                )
                span.set_model(result["model"], temperature=0.7)
                
                # Initialize agent memory
                memory = span.memory_tracker(namespace="agent_state")
                memory["goal"] = "Research transformer architectures"
                memory["status"] = "planning"
                memory["plan"] = result["text"]
                
                print(f"   Plan: {result['text'][:60]}...")

            # ─── Step 2: Research (tool use) ───
            print("🔧 Step 2: Researching...")
            with trace.span("web_search", operation_type="TOOL") as span:
                search_result = simulate_search_tool("transformer architecture attention")
                
                span.add_tool_call(
                    tool_name="search_web",
                    arguments={"query": "transformer architecture attention"},
                    result=search_result,
                    success=True,
                    latency_ms=120,
                )
                
                # Update memory with findings
                memory = span.memory_tracker(
                    initial_state={"goal": "Research transformer architectures", "status": "planning"},
                    namespace="agent_state",
                )
                memory["status"] = "researching"
                memory["sources"] = [r["url"] for r in search_result["results"]]
                
                print(f"   Found {search_result['total_results']} results")

            # ─── Step 3: Analysis (LLM) ───
            print("🤖 Step 3: Analyzing...")
            with trace.span("analysis_call", operation_type="LLM") as span:
                prompt = "Summarize the key research findings about transformer architectures"
                result = simulate_llm_call(prompt)
                
                span.set_prompt(prompt)
                span.set_completion(
                    result["text"],
                    prompt_tokens=result["prompt_tokens"],
                    completion_tokens=result["completion_tokens"],
                )
                span.set_model(result["model"], temperature=0.3)
                
                # Update memory with analysis
                memory = span.memory_tracker(
                    initial_state={
                        "goal": "Research transformer architectures",
                        "status": "researching",
                    },
                    namespace="agent_state",
                )
                memory["status"] = "analyzing"
                memory["key_findings"] = result["text"]
                memory["confidence"] = "high"
                
                print(f"   Analysis: {result['text'][:60]}...")

            # ─── Step 4: Memory Update ───
            print("🧠 Step 4: Updating knowledge base...")
            with trace.span("knowledge_update", operation_type="MEMORY") as span:
                memory = span.memory_tracker(
                    initial_state={
                        "goal": "Research transformer architectures",
                        "status": "analyzing",
                        "key_findings": "Attention is all you need...",
                        "confidence": "high",
                    },
                    namespace="agent_state",
                )
                
                memory["status"] = "completed"
                memory["summary"] = "Transformers use self-attention for parallel sequence processing"
                del memory["confidence"]  # Clean up temporary state
                memory["completed_at"] = time.strftime("%Y-%m-%d %H:%M:%S")
                
                print("   Knowledge base updated!")

            # ─── Step 5: Nested spans (demonstrating hierarchy) ───
            print("📊 Step 5: Generating report...")
            with trace.span("report_generation", operation_type="PLANNING") as parent_span:
                
                with trace.span("format_findings", operation_type="LLM") as child_span:
                    prompt = "Research the latest findings and create a summary report"
                    result = simulate_llm_call(prompt)
                    child_span.set_prompt(prompt)
                    child_span.set_completion(
                        result["text"],
                        prompt_tokens=result["prompt_tokens"],
                        completion_tokens=result["completion_tokens"],
                    )
                    child_span.set_model("gpt-4-turbo")
                
                with trace.span("save_report", operation_type="TOOL") as child_span:
                    child_span.add_tool_call(
                        tool_name="save_file",
                        arguments={"path": "report.md", "content": "# Research Report\n..."},
                        result={"success": True, "path": "report.md"},
                        success=True,
                        latency_ms=15,
                    )

        print("\n✅ Agent execution complete!")
        print(f"   Messages sent: {tracer.transport.messages_sent}")
        print(f"   Messages dropped: {tracer.transport.messages_dropped}")
        print(f"\n📺 View traces with: oculo-tui")
        print(f"📊 Analyze with: oculo analyze --trace <trace-id>")


if __name__ == "__main__":
    main()
