import { seedReviewRounds } from "../data/review-rounds";
import { seededQuery } from "./seeded";

export const reviewRoundsQuery = seededQuery("review-rounds", seedReviewRounds);
