from fastapi import FastAPI
from pydantic import BaseModel
from typing import List
import uvicorn
import numpy as np
from sentence_transformers import SentenceTransformer

app = FastAPI()

# Load sentence transformer model for semantic embeddings
# Using a lightweight, fast model suitable for reranking
# all-MiniLM-L6-v2 is a good balance of speed and quality
model = SentenceTransformer('all-MiniLM-L6-v2')

class Candidate(BaseModel):
    id: str
    title: str
    body: str

class RerankIn(BaseModel):
    query: str
    candidates: List[Candidate]

class RerankOut(BaseModel):
    order: List[str]

def cosine_similarity(a: np.ndarray, b: np.ndarray) -> float:
    """Compute cosine similarity between two vectors."""
    return np.dot(a, b) / (np.linalg.norm(a) * np.linalg.norm(b))

@app.post("/rerank", response_model=RerankOut)
async def rerank(inp: RerankIn):
    """
    Rerank candidates using semantic embeddings from sentence-transformers.
    This provides much better search quality than bag-of-words.
    """
    # Encode query into semantic embedding
    query_embedding = model.encode(inp.query, convert_to_numpy=True, normalize_embeddings=True)
    
    # Prepare candidate texts (title + body)
    candidate_texts = [f"{c.title} {c.body}" for c in inp.candidates]
    
    # Encode all candidates in batch for efficiency
    candidate_embeddings = model.encode(
        candidate_texts,
        convert_to_numpy=True,
        normalize_embeddings=True,
        show_progress_bar=False
    )
    
    # Compute cosine similarity between query and each candidate
    scored = []
    for i, candidate in enumerate(inp.candidates):
        similarity = cosine_similarity(query_embedding, candidate_embeddings[i])
        scored.append((candidate.id, float(similarity)))
    
    # Sort by similarity score (descending)
    scored.sort(key=lambda x: x[1], reverse=True)
    
    # Return ordered list of candidate IDs
    ordered = [sid for sid, _ in scored]
    return RerankOut(order=ordered)

@app.get("/health")
async def health():
    """Health check endpoint."""
    return {"status": "healthy", "model": "all-MiniLM-L6-v2"}

if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8000)
